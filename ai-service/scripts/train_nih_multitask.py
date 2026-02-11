import argparse
import csv
import hashlib
import json
import random
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Sequence, Tuple, Optional

import numpy as np
import torch
from PIL import Image
from sklearn.metrics import roc_auc_score
from torch import nn
from torch.utils.data import DataLoader, Dataset
from torchvision import models, transforms
from tqdm import tqdm


CLASS_NAMES = ["Pneumonia", "Nodule", "Mass", "Lung Opacity"]
OPACITY_PROXY_LABELS = {
    "Infiltration",
    "Consolidation",
    "Edema",
    "Atelectasis",
    "Effusion",
    "Pneumonia",
}


@dataclass
class Sample:
    image_path: Path
    patient_id: str
    target: np.ndarray


class NihCxrDataset(Dataset):
    def __init__(self, samples: Sequence[Sample], image_size: int) -> None:
        self.samples = list(samples)
        self.transform = transforms.Compose(
            [
                transforms.Resize((image_size, image_size)),
                transforms.ToTensor(),
            ]
        )

    def __len__(self) -> int:
        return len(self.samples)

    def __getitem__(self, index: int) -> Tuple[torch.Tensor, torch.Tensor]:
        sample = self.samples[index]
        img = Image.open(sample.image_path).convert("L")
        x = self.transform(img)  # [1, H, W] in [0,1]
        y = torch.from_numpy(sample.target.astype(np.float32))
        return x, y


def parse_labels(raw_labels: str) -> np.ndarray:
    labels = {x.strip() for x in raw_labels.split("|") if x.strip()}
    pneumonia = 1.0 if "Pneumonia" in labels else 0.0
    nodule = 1.0 if "Nodule" in labels else 0.0
    mass = 1.0 if "Mass" in labels else 0.0
    opacity = 1.0 if labels.intersection(OPACITY_PROXY_LABELS) else 0.0
    return np.array([pneumonia, nodule, mass, opacity], dtype=np.float32)


def deterministic_bucket(value: str, seed: int) -> float:
    digest = hashlib.sha1(f"{seed}:{value}".encode("utf-8")).hexdigest()
    return int(digest[:8], 16) / 0xFFFFFFFF


def split_by_patient(
    samples: Sequence[Sample], val_ratio: float, seed: int
) -> Tuple[List[Sample], List[Sample]]:
    train: List[Sample] = []
    val: List[Sample] = []
    for sample in samples:
        b = deterministic_bucket(sample.patient_id, seed)
        if b < val_ratio:
            val.append(sample)
        else:
            train.append(sample)
    return train, val


def split_by_image_random(
    samples: Sequence[Sample], val_ratio: float, seed: int
) -> Tuple[List[Sample], List[Sample]]:
    if len(samples) < 2:
        return list(samples), []
    rng = random.Random(seed)
    items = list(samples)
    rng.shuffle(items)
    n_val = max(1, int(len(items) * val_ratio))
    n_val = min(n_val, max(len(items) - 1, 1))
    val = items[:n_val]
    train = items[n_val:]
    if not train:
        train = items[:-1]
        val = items[-1:]
    return train, val


def find_image_paths(dataset_root: Path) -> Dict[str, Path]:
    candidates = [p for p in dataset_root.rglob("*") if p.is_file() and p.suffix.lower() in {".png", ".jpg", ".jpeg"}]
    return {p.name: p for p in candidates}


def load_samples(dataset_root: Path, csv_path: Path) -> List[Sample]:
    image_map = find_image_paths(dataset_root)
    samples: List[Sample] = []
    with csv_path.open("r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        for row in reader:
            image_name = (row.get("Image Index") or "").strip()
            labels = (row.get("Finding Labels") or "").strip()
            patient_id = (row.get("Patient ID") or "").strip()
            if not image_name or image_name not in image_map:
                continue
            if not patient_id:
                patient_id = image_name.split("_")[0]
            target = parse_labels(labels)
            samples.append(
                Sample(
                    image_path=image_map[image_name],
                    patient_id=patient_id,
                    target=target,
                )
            )
    return samples


def resolve_csv_path(dataset_root: Path, explicit_csv: str) -> Path:
    if explicit_csv:
        p = Path(explicit_csv).expanduser().resolve()
        if not p.exists():
            raise FileNotFoundError(f"CSV not found at explicit path: {p}")
        return p

    direct_candidates = [
        dataset_root / "Data_Entry_2017.csv",
        dataset_root / "Data_Entry_2017_v2020.csv",
        dataset_root / "metadata" / "Data_Entry_2017.csv",
        dataset_root / "metadata" / "Data_Entry_2017_v2020.csv",
    ]
    for p in direct_candidates:
        if p.exists():
            return p

    recursive_candidates = sorted(
        [p for p in dataset_root.rglob("Data_Entry_2017*.csv") if p.is_file()]
    )
    if recursive_candidates:
        return recursive_candidates[0]

    raise FileNotFoundError(
        "CSV not found. Searched:\n"
        f"- {dataset_root / 'Data_Entry_2017.csv'}\n"
        f"- {dataset_root / 'Data_Entry_2017_v2020.csv'}\n"
        f"- {dataset_root / 'metadata' / 'Data_Entry_2017.csv'}\n"
        f"- {dataset_root / 'metadata' / 'Data_Entry_2017_v2020.csv'}\n"
        f"- recursive pattern: {dataset_root}\\**\\Data_Entry_2017*.csv\n"
        "You can also pass --csv-path explicitly."
    )


def build_model() -> nn.Module:
    model = models.densenet121(weights=models.DenseNet121_Weights.IMAGENET1K_V1)
    old_conv = model.features.conv0
    new_conv = nn.Conv2d(
        1,
        old_conv.out_channels,
        kernel_size=old_conv.kernel_size,
        stride=old_conv.stride,
        padding=old_conv.padding,
        bias=old_conv.bias is not None,
    )
    with torch.no_grad():
        new_conv.weight.copy_(old_conv.weight.mean(dim=1, keepdim=True))
        if old_conv.bias is not None and new_conv.bias is not None:
            new_conv.bias.copy_(old_conv.bias)
    model.features.conv0 = new_conv
    in_features = model.classifier.in_features
    model.classifier = nn.Linear(in_features, len(CLASS_NAMES))
    return model


def compute_pos_weight(samples: Sequence[Sample]) -> torch.Tensor:
    y = np.stack([s.target for s in samples], axis=0)
    pos = y.sum(axis=0)
    neg = y.shape[0] - pos
    w = (neg + 1.0) / (pos + 1.0)
    return torch.tensor(w, dtype=torch.float32)


def evaluate(model: nn.Module, loader: DataLoader, device: torch.device) -> Dict[str, float]:
    model.eval()
    ys: List[np.ndarray] = []
    ps: List[np.ndarray] = []
    loss_sum = 0.0
    n = 0
    criterion = nn.BCEWithLogitsLoss()
    with torch.no_grad():
        for x, y in loader:
            x = x.to(device)
            y = y.to(device)
            logits = model(x)
            loss = criterion(logits, y)
            loss_sum += float(loss.item()) * x.shape[0]
            n += x.shape[0]
            ys.append(y.cpu().numpy())
            ps.append(torch.sigmoid(logits).cpu().numpy())

    y_true = np.concatenate(ys, axis=0)
    y_prob = np.concatenate(ps, axis=0)
    metrics: Dict[str, float] = {"val_loss": loss_sum / max(n, 1)}
    aucs: List[float] = []
    for i, name in enumerate(CLASS_NAMES):
        try:
            auc = float(roc_auc_score(y_true[:, i], y_prob[:, i]))
        except ValueError:
            auc = float("nan")
        metrics[f"auc_{name}"] = auc
        if not np.isnan(auc):
            aucs.append(auc)
    metrics["mean_auc"] = float(np.mean(aucs)) if aucs else 0.0
    return metrics


def train(args: argparse.Namespace) -> None:
    random.seed(args.seed)
    np.random.seed(args.seed)
    torch.manual_seed(args.seed)

    dataset_root = Path(args.dataset_root).resolve()
    csv_path = resolve_csv_path(dataset_root, args.csv_path)
    output_dir = Path(args.output_dir).resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    # Quick sanity check: full NIH metadata should have ~112k rows.
    # Very small CSV usually means a sample/demo file, not the full dataset labels.
    with csv_path.open("r", encoding="utf-8") as f:
        row_count = sum(1 for _ in f) - 1
    if row_count < args.min_csv_rows:
        msg = (
            f"CSV rows={row_count} is suspiciously small. "
            "This usually indicates a sample metadata file, not full NIH labels. "
            "Please use full Data_Entry_2017(.csv or _v2020.csv), "
            "or pass --allow-small-csv to continue anyway."
        )
        if not args.allow_small_csv:
            raise RuntimeError(msg)
        print(f"[warn] {msg}")

    samples = load_samples(dataset_root, csv_path)
    if not samples:
        raise RuntimeError("No training samples found. Check dataset_root and CSV path.")
    if len(samples) < 2:
        raise RuntimeError(
            "Not enough samples to split train/val (need >=2). "
            "Check CSV/image matching and dataset completeness."
        )

    unique_patients = len({s.patient_id for s in samples})
    print(
        f"[info] Loaded samples={len(samples)}, unique_patients={unique_patients}, "
        f"val_ratio={args.val_ratio}"
    )

    train_samples, val_samples = split_by_patient(samples, val_ratio=args.val_ratio, seed=args.seed)
    if not train_samples or not val_samples:
        if args.fallback_image_split:
            print("[warn] Patient-level split is empty; falling back to image-level random split.")
            train_samples, val_samples = split_by_image_random(samples, val_ratio=args.val_ratio, seed=args.seed)
        else:
            raise RuntimeError("Train/val split is empty; adjust val_ratio or dataset.")

    if not train_samples or not val_samples:
        raise RuntimeError("Train/val split is still empty after fallback; dataset too small.")

    train_ds = NihCxrDataset(train_samples, image_size=args.image_size)
    val_ds = NihCxrDataset(val_samples, image_size=args.image_size)
    train_loader = DataLoader(train_ds, batch_size=args.batch_size, shuffle=True, num_workers=args.num_workers)
    val_loader = DataLoader(val_ds, batch_size=args.batch_size, shuffle=False, num_workers=args.num_workers)

    device = torch.device("cuda" if torch.cuda.is_available() and not args.cpu else "cpu")
    model = build_model().to(device)

    pos_weight = compute_pos_weight(train_samples).to(device)
    criterion = nn.BCEWithLogitsLoss(pos_weight=pos_weight)
    optimizer = torch.optim.AdamW(model.parameters(), lr=args.lr, weight_decay=args.weight_decay)
    scheduler = torch.optim.lr_scheduler.CosineAnnealingLR(optimizer, T_max=max(args.epochs, 1))

    best_auc = -1.0
    best_path = output_dir / "nih_multitask_best.pt"
    last_path = output_dir / "nih_multitask_last.pt"
    history: List[Dict[str, float]] = []

    for epoch in range(1, args.epochs + 1):
        model.train()
        running = 0.0
        seen = 0
        pbar = tqdm(train_loader, desc=f"Epoch {epoch}/{args.epochs}", ncols=100)
        for x, y in pbar:
            x = x.to(device)
            y = y.to(device)
            optimizer.zero_grad(set_to_none=True)
            logits = model(x)
            loss = criterion(logits, y)
            loss.backward()
            optimizer.step()
            running += float(loss.item()) * x.shape[0]
            seen += x.shape[0]
            pbar.set_postfix({"train_loss": running / max(seen, 1)})

        scheduler.step()
        metrics = evaluate(model, val_loader, device)
        metrics["epoch"] = float(epoch)
        metrics["train_loss"] = running / max(seen, 1)
        history.append(metrics)
        print(json.dumps(metrics, ensure_ascii=False))

        ckpt = {
            "state_dict": model.state_dict(),
            "class_names": CLASS_NAMES,
            "image_size": args.image_size,
            "metrics": metrics,
        }
        torch.save(ckpt, last_path)
        if metrics["mean_auc"] > best_auc:
            best_auc = metrics["mean_auc"]
            torch.save(ckpt, best_path)

    summary = {
        "dataset_root": str(dataset_root),
        "csv_path": str(csv_path),
        "train_size": len(train_samples),
        "val_size": len(val_samples),
        "best_mean_auc": best_auc,
        "best_checkpoint": str(best_path),
        "last_checkpoint": str(last_path),
        "history": history,
    }
    (output_dir / "nih_multitask_summary.json").write_text(
        json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8"
    )
    print(f"Training done. Best mean AUC={best_auc:.4f}")
    print(f"Best checkpoint: {best_path}")


def build_argparser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(description="Train NIH ChestXray14 multitask model for current AI service.")
    p.add_argument("--dataset-root", required=True, help="NIH dataset root containing images and Data_Entry_2017.csv")
    p.add_argument("--csv-path", default="", help="Optional path to Data_Entry_2017.csv")
    p.add_argument("--output-dir", default="../models", help="Output dir for checkpoints")
    p.add_argument("--image-size", type=int, default=224)
    p.add_argument("--batch-size", type=int, default=32)
    p.add_argument("--epochs", type=int, default=8)
    p.add_argument("--lr", type=float, default=1e-4)
    p.add_argument("--weight-decay", type=float, default=1e-4)
    p.add_argument("--val-ratio", type=float, default=0.15)
    p.add_argument("--num-workers", type=int, default=4)
    p.add_argument("--seed", type=int, default=42)
    p.add_argument("--cpu", action="store_true")
    p.add_argument(
        "--fallback-image-split",
        dest="fallback_image_split",
        action="store_true",
        default=True,
        help="Fallback to image-level random split when patient-level split is empty.",
    )
    p.add_argument(
        "--no-fallback-image-split",
        dest="fallback_image_split",
        action="store_false",
        help="Disable fallback and fail fast on patient-level split empty.",
    )
    p.add_argument("--allow-small-csv", action="store_true")
    p.add_argument("--min-csv-rows", type=int, default=1000)
    return p


if __name__ == "__main__":
    train(build_argparser().parse_args())
