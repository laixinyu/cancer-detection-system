import argparse
import csv
import json
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Iterable, List, Sequence, Tuple

import numpy as np
import torch
from PIL import Image
from torch.utils.data import DataLoader, Dataset

SCRIPT_DIR = Path(__file__).resolve().parent
AI_SERVICE_ROOT = SCRIPT_DIR.parent
if str(AI_SERVICE_ROOT) not in sys.path:
    sys.path.insert(0, str(AI_SERVICE_ROOT))
if str(SCRIPT_DIR) not in sys.path:
    sys.path.insert(0, str(SCRIPT_DIR))

from app.preprocess import ImagePreprocessor
from train_nih_multitask import (
    CLASS_NAMES,
    Sample,
    build_model,
    build_transforms,
    choose_device,
    csv_sanity_check,
    load_checkpoint_weights,
    load_samples,
    resolve_csv_path,
    resolve_num_workers,
    split_samples_nih_official,
    split_samples_with_fallback,
)


TASK_PNEUMONIA = "pneumonia"
TASK_LESION = "lesion"
LABEL_PNEUMONIA = "Pneumonia"
LABEL_LESION = "Lesion(Nodule|Mass)"


@dataclass
class SplitBundle:
    split_name: str
    samples: Sequence[Sample]


class EvalDataset(Dataset):
    def __init__(self, samples: Sequence[Sample], image_size: int) -> None:
        self.samples = list(samples)
        self.transform = build_transforms(image_size=image_size, augment=False)

    def __len__(self) -> int:
        return len(self.samples)

    def __getitem__(self, index: int):
        sample = self.samples[index]
        raw = sample.image_path.read_bytes()
        gray = ImagePreprocessor.read_grayscale(raw)
        img = Image.fromarray(gray, mode="L")
        x = self.transform(img)
        y = torch.from_numpy(sample.target.astype(np.float32))
        return x, y, index


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Export validation/test predictions for threshold tuning and calibration."
    )
    p.add_argument("--dataset-root", required=True, help="NIH dataset root")
    p.add_argument("--csv-path", default="", help="Optional metadata CSV path")
    p.add_argument("--checkpoint", required=True, help="Trained checkpoint path")
    p.add_argument(
        "--split-mode",
        choices=["hash_patient", "nih_official"],
        default="nih_official",
        help="Split mode used to reproduce train/val/test partitions",
    )
    p.add_argument(
        "--target-split",
        choices=["val", "test", "both"],
        default="both",
        help="Which split to export",
    )
    p.add_argument("--val-ratio", type=float, default=0.15)
    p.add_argument("--seed", type=int, default=42)
    p.add_argument("--fallback-image-split", action="store_true", default=True)
    p.add_argument("--disable-fallback-image-split", action="store_true")
    p.add_argument("--min-csv-rows", type=int, default=1000)
    p.add_argument("--allow-small-csv", action="store_true")
    p.add_argument("--batch-size", type=int, default=64)
    p.add_argument("--num-workers", type=int, default=-1)
    p.add_argument("--cpu", action="store_true")
    p.add_argument("--image-size", type=int, default=0, help="Optional override; default from checkpoint")
    p.add_argument("--backbone", default="", help="Optional override; default from checkpoint")
    p.add_argument("--out", required=True, help="Output CSV path")
    return p.parse_args()


def resolve_model_meta(args: argparse.Namespace, checkpoint_path: Path) -> Tuple[str, int]:
    ckpt = torch.load(checkpoint_path, map_location="cpu")
    checkpoint_backbone = str(ckpt.get("backbone", "")).strip()
    checkpoint_image_size = int(ckpt.get("image_size", 0) or 0)

    backbone = args.backbone.strip() or checkpoint_backbone or "efficientnet_v2_s"
    image_size = int(args.image_size) if int(args.image_size) > 0 else checkpoint_image_size
    if image_size <= 0:
        image_size = 320
    return backbone, image_size


def resolve_splits(
    samples: Sequence[Sample], dataset_root: Path, args: argparse.Namespace
) -> List[SplitBundle]:
    if args.disable_fallback_image_split:
        args.fallback_image_split = False

    if args.split_mode == "nih_official":
        _, val_samples, test_samples, _ = split_samples_nih_official(samples, dataset_root, args)
    else:
        _, val_samples = split_samples_with_fallback(samples, args)
        test_samples = []

    bundles: List[SplitBundle] = []
    if args.target_split in {"val", "both"}:
        bundles.append(SplitBundle(split_name="val", samples=val_samples))
    if args.target_split in {"test", "both"} and test_samples:
        bundles.append(SplitBundle(split_name="test", samples=test_samples))
    return bundles


def batched_inference(
    model: torch.nn.Module, loader: DataLoader, device: torch.device
) -> Tuple[np.ndarray, np.ndarray, np.ndarray]:
    model.eval()
    logits_blocks: List[np.ndarray] = []
    probs_blocks: List[np.ndarray] = []
    y_blocks: List[np.ndarray] = []

    with torch.no_grad():
        for x, y, _ in loader:
            x = x.to(device, non_blocking=True)
            logits = model(x)
            probs = torch.sigmoid(logits)
            logits_blocks.append(logits.detach().cpu().numpy())
            probs_blocks.append(probs.detach().cpu().numpy())
            y_blocks.append(y.detach().cpu().numpy())

    logits_all = np.concatenate(logits_blocks, axis=0) if logits_blocks else np.zeros((0, len(CLASS_NAMES)))
    probs_all = np.concatenate(probs_blocks, axis=0) if probs_blocks else np.zeros((0, len(CLASS_NAMES)))
    y_all = np.concatenate(y_blocks, axis=0) if y_blocks else np.zeros((0, len(CLASS_NAMES)))
    return logits_all, probs_all, y_all


def iter_rows(
    split_name: str,
    samples: Sequence[Sample],
    logits: np.ndarray,
    probs: np.ndarray,
    backbone: str,
    image_size: int,
    checkpoint_path: Path,
) -> Iterable[List[object]]:
    idx_pneumonia = CLASS_NAMES.index("Pneumonia")
    idx_nodule = CLASS_NAMES.index("Nodule")
    idx_mass = CLASS_NAMES.index("Mass")

    for i, sample in enumerate(samples):
        y_true = sample.target.astype(np.int32)
        p = probs[i]
        l = logits[i]

        pneumonia_true = int(y_true[idx_pneumonia])
        pneumonia_score = float(p[idx_pneumonia])
        pneumonia_logit = float(l[idx_pneumonia])
        yield [
            split_name,
            sample.image_path.name,
            sample.patient_id,
            TASK_PNEUMONIA,
            LABEL_PNEUMONIA,
            pneumonia_true,
            pneumonia_score,
            pneumonia_logit,
            backbone,
            image_size,
            str(checkpoint_path),
        ]

        lesion_true = int(max(y_true[idx_nodule], y_true[idx_mass]))
        lesion_score = float(max(p[idx_nodule], p[idx_mass]))
        lesion_logit = float(max(l[idx_nodule], l[idx_mass]))
        yield [
            split_name,
            sample.image_path.name,
            sample.patient_id,
            TASK_LESION,
            LABEL_LESION,
            lesion_true,
            lesion_score,
            lesion_logit,
            backbone,
            image_size,
            str(checkpoint_path),
        ]


def main() -> None:
    args = parse_args()
    dataset_root = Path(args.dataset_root).expanduser().resolve()
    if not dataset_root.exists():
        raise FileNotFoundError(f"dataset root not found: {dataset_root}")

    checkpoint_path = Path(args.checkpoint).expanduser().resolve()
    if not checkpoint_path.exists():
        raise FileNotFoundError(f"checkpoint not found: {checkpoint_path}")

    csv_path = resolve_csv_path(dataset_root, args.csv_path)
    csv_sanity_check(csv_path, min_csv_rows=args.min_csv_rows, allow_small_csv=args.allow_small_csv)
    samples = load_samples(dataset_root, csv_path)
    if not samples:
        raise RuntimeError("No samples loaded from dataset/csv.")

    splits = resolve_splits(samples, dataset_root, args)
    if not splits:
        raise RuntimeError("No split selected for export.")

    backbone, image_size = resolve_model_meta(args, checkpoint_path)
    device = choose_device(args.cpu)
    model = build_model(backbone=backbone, dropout=0.0).to(device)
    load_checkpoint_weights(model, checkpoint_path, device=device)

    num_workers = resolve_num_workers(args.num_workers)
    out_path = Path(args.out).expanduser().resolve()
    out_path.parent.mkdir(parents=True, exist_ok=True)

    header = [
        "split",
        "image_name",
        "patient_id",
        "task",
        "label",
        "y_true",
        "y_score",
        "y_logit",
        "backbone",
        "image_size",
        "checkpoint",
    ]

    rows_written = 0
    with out_path.open("w", encoding="utf-8", newline="") as f:
        writer = csv.writer(f)
        writer.writerow(header)
        for bundle in splits:
            ds = EvalDataset(samples=bundle.samples, image_size=image_size)
            loader = DataLoader(
                ds,
                batch_size=args.batch_size,
                shuffle=False,
                num_workers=num_workers,
                pin_memory=device.type == "cuda",
                persistent_workers=num_workers > 0,
            )
            logits, probs, _ = batched_inference(model=model, loader=loader, device=device)
            for row in iter_rows(
                split_name=bundle.split_name,
                samples=bundle.samples,
                logits=logits,
                probs=probs,
                backbone=backbone,
                image_size=image_size,
                checkpoint_path=checkpoint_path,
            ):
                writer.writerow(row)
                rows_written += 1

    summary = {
        "rows_written": rows_written,
        "out": str(out_path),
        "dataset_root": str(dataset_root),
        "csv_path": str(csv_path),
        "checkpoint": str(checkpoint_path),
        "backbone": backbone,
        "image_size": image_size,
        "target_split": args.target_split,
        "split_mode": args.split_mode,
    }
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
