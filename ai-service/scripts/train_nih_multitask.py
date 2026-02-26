import argparse
import csv
import hashlib
import json
import os
import random
import time
import sys
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, Iterable, List, Sequence, Tuple, Optional

import numpy as np
import torch
from PIL import Image
from torch import nn
from torch.utils.data import DataLoader, Dataset, WeightedRandomSampler
from torchvision import models, transforms
from tqdm import tqdm

SCRIPT_DIR = Path(__file__).resolve().parent
AI_SERVICE_ROOT = SCRIPT_DIR.parent
if str(AI_SERVICE_ROOT) not in sys.path:
    sys.path.insert(0, str(AI_SERVICE_ROOT))

from app.preprocess import ImagePreprocessor


CLASS_NAMES = ["Pneumonia", "Nodule", "Mass", "Lung Opacity"]
BACKBONE_CHOICES = [
    "densenet121",
    "efficientnet_b0",
    "efficientnet_v2_s",
    "mobilenet_v3_small",
    "convnext_tiny",
    "convnext_large",
    "swin_b",
    "vit_b_16",
]
SPLIT_MODE_CHOICES = ["hash_patient", "nih_official"]
LR_SCHEDULER_CHOICES = ["cosine", "plateau", "none"]
OPACITY_PROXY_LABELS = {
    "Infiltration",
    "Consolidation",
    "Edema",
    "Atelectasis",
    "Effusion",
    "Pneumonia",
}


def build_grad_scaler(device_type: str, enabled: bool):
    if not (hasattr(torch, "amp") and hasattr(torch.amp, "GradScaler")):
        raise RuntimeError("Current PyTorch version does not support torch.amp.GradScaler")
    return torch.amp.GradScaler(device_type, enabled=enabled)


def amp_autocast(device_type: str, enabled: bool):
    if not (hasattr(torch, "amp") and hasattr(torch.amp, "autocast")):
        raise RuntimeError("Current PyTorch version does not support torch.amp.autocast")
    return torch.amp.autocast(device_type=device_type, enabled=enabled)


@dataclass
class Sample:
    image_path: Path
    patient_id: str
    target: np.ndarray


class NihCxrDataset(Dataset):
    def __init__(self, samples: Sequence[Sample], transform: transforms.Compose) -> None:
        self.samples = list(samples)
        self.transform = transform

    def __len__(self) -> int:
        return len(self.samples)

    def __getitem__(self, index: int) -> Tuple[torch.Tensor, torch.Tensor]:
        sample = self.samples[index]
        raw = sample.image_path.read_bytes()
        gray = ImagePreprocessor.read_grayscale(raw)
        img = Image.fromarray(gray, mode="L")
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


def parse_class_loss_weights(raw: str) -> List[float]:
    values = [x.strip() for x in raw.split(",") if x.strip()]
    if not values:
        return [1.0] * len(CLASS_NAMES)
    if len(values) != len(CLASS_NAMES):
        raise ValueError(f"--class-loss-weights expects {len(CLASS_NAMES)} values, got {len(values)}")
    weights = [float(x) for x in values]
    if any(w <= 0 for w in weights):
        raise ValueError("--class-loss-weights values must be > 0")
    return weights


class FocalWithLogitsLoss(nn.Module):
    def __init__(
        self,
        gamma: float = 2.0,
        alpha: float = 0.25,
        pos_weight: Optional[torch.Tensor] = None,
        class_weights: Optional[torch.Tensor] = None,
    ) -> None:
        super().__init__()
        self.gamma = gamma
        self.alpha = alpha
        self.pos_weight = pos_weight
        self.class_weights = class_weights

    def forward(self, logits: torch.Tensor, targets: torch.Tensor) -> torch.Tensor:
        bce = nn.functional.binary_cross_entropy_with_logits(
            logits, targets, reduction="none", pos_weight=self.pos_weight
        )
        probs = torch.sigmoid(logits)
        p_t = probs * targets + (1.0 - probs) * (1.0 - targets)
        alpha_factor = self.alpha * targets + (1.0 - self.alpha) * (1.0 - targets)
        focal_factor = (1.0 - p_t).pow(self.gamma)
        loss = alpha_factor * focal_factor * bce
        if self.class_weights is not None:
            loss = loss * self.class_weights.view(1, -1)
        return loss.mean()


class BCEWithLogitsWeightedLoss(nn.Module):
    def __init__(self, pos_weight: Optional[torch.Tensor] = None, class_weights: Optional[torch.Tensor] = None) -> None:
        super().__init__()
        self.pos_weight = pos_weight
        self.class_weights = class_weights

    def forward(self, logits: torch.Tensor, targets: torch.Tensor) -> torch.Tensor:
        loss = nn.functional.binary_cross_entropy_with_logits(
            logits, targets, reduction="none", pos_weight=self.pos_weight
        )
        if self.class_weights is not None:
            loss = loss * self.class_weights.view(1, -1)
        return loss.mean()


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


def adapt_conv2d_in_channels(old_conv: nn.Conv2d, in_channels: int = 1) -> nn.Conv2d:
    new_conv = nn.Conv2d(
        in_channels,
        old_conv.out_channels,
        kernel_size=old_conv.kernel_size,
        stride=old_conv.stride,
        padding=old_conv.padding,
        dilation=old_conv.dilation,
        groups=old_conv.groups,
        bias=old_conv.bias is not None,
    )
    with torch.no_grad():
        new_conv.weight.copy_(old_conv.weight.mean(dim=1, keepdim=True).repeat(1, in_channels, 1, 1))
        if old_conv.bias is not None and new_conv.bias is not None:
            new_conv.bias.copy_(old_conv.bias)
    return new_conv


def build_model(backbone: str, dropout: float = 0.0) -> nn.Module:
    if backbone == "densenet121":
        model = models.densenet121(weights=models.DenseNet121_Weights.IMAGENET1K_V1)
        model.features.conv0 = adapt_conv2d_in_channels(model.features.conv0, in_channels=1)
        in_features = model.classifier.in_features
        head: nn.Module = nn.Linear(in_features, len(CLASS_NAMES))
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier = head
        return model

    if backbone == "efficientnet_b0":
        model = models.efficientnet_b0(weights=models.EfficientNet_B0_Weights.IMAGENET1K_V1)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        head: nn.Module = nn.Linear(in_features, len(CLASS_NAMES))
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier[-1] = head
        return model

    if backbone == "efficientnet_v2_s":
        model = models.efficientnet_v2_s(weights=models.EfficientNet_V2_S_Weights.IMAGENET1K_V1)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        head: nn.Module = nn.Linear(in_features, len(CLASS_NAMES))
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier[-1] = head
        return model

    if backbone == "mobilenet_v3_small":
        model = models.mobilenet_v3_small(weights=models.MobileNet_V3_Small_Weights.IMAGENET1K_V1)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        head: nn.Module = nn.Linear(in_features, len(CLASS_NAMES))
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier[-1] = head
        return model

    if backbone == "convnext_tiny":
        model = models.convnext_tiny(weights=models.ConvNeXt_Tiny_Weights.IMAGENET1K_V1)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        head: nn.Module = nn.Linear(in_features, len(CLASS_NAMES))
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier[-1] = head
        return model

    if backbone == "convnext_large":
        model = models.convnext_large(weights=models.ConvNeXt_Large_Weights.IMAGENET1K_V1)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        head: nn.Module = nn.Linear(in_features, len(CLASS_NAMES))
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier[-1] = head
        return model

    if backbone == "swin_b":
        model = models.swin_b(weights=models.Swin_B_Weights.IMAGENET1K_V1)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.head.in_features
        head: nn.Module = nn.Linear(in_features, len(CLASS_NAMES))
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.head = head
        return model

    if backbone == "vit_b_16":
        model = models.vit_b_16(weights=models.ViT_B_16_Weights.IMAGENET1K_V1)
        model.conv_proj = adapt_conv2d_in_channels(model.conv_proj, in_channels=1)
        in_features = model.heads.head.in_features
        head: nn.Module = nn.Linear(in_features, len(CLASS_NAMES))
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.heads.head = head
        return model

    raise ValueError(f"Unsupported backbone: {backbone}")


def load_checkpoint_weights(model: nn.Module, checkpoint_path: Path, device: torch.device) -> None:
    ckpt = torch.load(checkpoint_path, map_location=device)
    state_dict = ckpt.get("state_dict", ckpt)
    model.load_state_dict(state_dict, strict=True)


def distillation_bce_loss(student_logits: torch.Tensor, teacher_logits: torch.Tensor, temperature: float) -> torch.Tensor:
    t = max(float(temperature), 1e-6)
    with torch.no_grad():
        teacher_prob = torch.sigmoid(teacher_logits / t)
    return nn.functional.binary_cross_entropy_with_logits(student_logits / t, teacher_prob) * (t * t)


def resolve_teacher_backbone(student_backbone: str, teacher_backbone: str) -> str:
    return teacher_backbone if teacher_backbone else student_backbone


def create_teacher_model(args: argparse.Namespace, device: torch.device) -> Optional[nn.Module]:
    if not args.teacher_checkpoint:
        return None
    teacher_ckpt = Path(args.teacher_checkpoint).expanduser().resolve()
    if not teacher_ckpt.exists():
        raise FileNotFoundError(f"Teacher checkpoint not found: {teacher_ckpt}")
    ckpt = torch.load(teacher_ckpt, map_location=device)
    teacher_backbone = str(ckpt.get("backbone") or resolve_teacher_backbone(args.backbone, args.teacher_backbone))
    teacher_dropout = float(ckpt.get("dropout", 0.0))
    teacher = build_model(teacher_backbone, dropout=teacher_dropout).to(device)
    state_dict = ckpt.get("state_dict", ckpt)
    teacher.load_state_dict(state_dict, strict=True)
    teacher.eval()
    for p in teacher.parameters():
        p.requires_grad = False
    print(
        f"[info] Distillation enabled. teacher_backbone={teacher_backbone}, "
        f"teacher_dropout={teacher_dropout}, teacher_ckpt={teacher_ckpt}"
    )
    return teacher


def validate_args(args: argparse.Namespace) -> None:
    if args.backbone not in BACKBONE_CHOICES:
        raise ValueError(f"Unsupported --backbone: {args.backbone}")
    if args.teacher_backbone and args.teacher_backbone not in BACKBONE_CHOICES:
        raise ValueError(f"Unsupported --teacher-backbone: {args.teacher_backbone}")
    if args.split_mode not in SPLIT_MODE_CHOICES:
        raise ValueError(f"Unsupported --split-mode: {args.split_mode}")
    if args.distill_alpha < 0.0 or args.distill_alpha > 1.0:
        raise ValueError("--distill-alpha must be in [0,1]")
    if args.distill_temp <= 0.0:
        raise ValueError("--distill-temp must be > 0")
    if args.loss_type not in {"bce", "focal"}:
        raise ValueError("--loss-type must be bce or focal")
    if args.focal_gamma < 0.0:
        raise ValueError("--focal-gamma must be >= 0")
    if args.focal_alpha < 0.0 or args.focal_alpha > 1.0:
        raise ValueError("--focal-alpha must be in [0,1]")
    if args.sampler_power <= 0.0:
        raise ValueError("--sampler-power must be > 0")
    if args.sampler_max_weight < 1.0:
        raise ValueError("--sampler-max-weight must be >= 1")
    if args.grad_clip < 0.0:
        raise ValueError("--grad-clip must be >= 0")
    if args.dropout < 0.0 or args.dropout >= 1.0:
        raise ValueError("--dropout must be in [0,1)")
    if args.early_stop_patience < 0:
        raise ValueError("--early-stop-patience must be >= 0")
    if args.early_stop_min_delta < 0.0:
        raise ValueError("--early-stop-min-delta must be >= 0")
    if args.augment_strength not in {"light", "strong"}:
        raise ValueError("--augment-strength must be light or strong")
    if args.num_workers < -1:
        raise ValueError("--num-workers must be >= -1")
    if args.prefetch_factor < 1:
        raise ValueError("--prefetch-factor must be >= 1")
    if args.lr_scheduler not in LR_SCHEDULER_CHOICES:
        raise ValueError(f"Unsupported --lr-scheduler: {args.lr_scheduler}")
    if args.lr_patience < 1:
        raise ValueError("--lr-patience must be >= 1")
    if args.lr_factor <= 0.0 or args.lr_factor >= 1.0:
        raise ValueError("--lr-factor must be in (0,1)")
    if args.lr_min < 0.0:
        raise ValueError("--lr-min must be >= 0")
    if args.batch_size_scale < 1.0:
        raise ValueError("--batch-size-scale must be >= 1.0")
    parse_class_loss_weights(args.class_loss_weights)


def format_train_postfix(supervised: float, distill: float, total: float, use_distill: bool) -> Dict[str, float]:
    if use_distill:
        return {"loss": total, "sup": supervised, "kd": distill}
    return {"loss": total}


def unpack_teacher_checkpoint_path(args: argparse.Namespace) -> str:
    if not args.teacher_checkpoint:
        return ""
    return str(Path(args.teacher_checkpoint).expanduser().resolve())


def should_use_distillation(teacher_model: Optional[nn.Module], distill_alpha: float) -> bool:
    return teacher_model is not None and distill_alpha > 0.0


def train_step_loss(
    student_logits: torch.Tensor,
    y: torch.Tensor,
    criterion: nn.Module,
    teacher_model: Optional[nn.Module],
    x: torch.Tensor,
    distill_alpha: float,
    distill_temp: float,
) -> Tuple[torch.Tensor, float, float]:
    supervised_loss = criterion(student_logits, y)
    distill_loss = torch.tensor(0.0, device=student_logits.device)
    if should_use_distillation(teacher_model, distill_alpha):
        with torch.no_grad():
            teacher_logits = teacher_model(x)
        distill_loss = distillation_bce_loss(student_logits, teacher_logits, temperature=distill_temp)
        total_loss = (1.0 - distill_alpha) * supervised_loss + distill_alpha * distill_loss
    else:
        total_loss = supervised_loss
    return total_loss, float(supervised_loss.item()), float(distill_loss.item())


def backbone_checkpoint_prefix(backbone: str) -> str:
    return backbone.replace("/", "_")


def build_checkpoint_paths(output_dir: Path, backbone: str) -> Tuple[Path, Path]:
    if backbone == "densenet121":
        best_path = output_dir / "nih_multitask_best.pt"
        last_path = output_dir / "nih_multitask_last.pt"
        return best_path, last_path
    prefix = backbone_checkpoint_prefix(backbone)
    best_path = output_dir / f"nih_multitask_{prefix}_best.pt"
    last_path = output_dir / f"nih_multitask_{prefix}_last.pt"
    return best_path, last_path


def build_teacher_info(args: argparse.Namespace) -> Dict[str, object]:
    return {
        "enabled": bool(args.teacher_checkpoint and args.distill_alpha > 0.0),
        "teacher_checkpoint": unpack_teacher_checkpoint_path(args),
        "teacher_backbone": resolve_teacher_backbone(args.backbone, args.teacher_backbone) if args.teacher_checkpoint else "",
        "distill_alpha": args.distill_alpha,
        "distill_temp": args.distill_temp,
    }


def training_banner(args: argparse.Namespace, train_size: int, val_size: int) -> None:
    print(
        f"[info] backbone={args.backbone}, image_size={args.image_size}, "
        f"train={train_size}, val={val_size}, batch_size={args.batch_size}, "
        f"lr_scheduler={args.lr_scheduler}"
    )


def model_metadata(args: argparse.Namespace) -> Dict[str, object]:
    return {
        "class_names": CLASS_NAMES,
        "image_size": args.image_size,
        "backbone": args.backbone,
        "loss_type": args.loss_type,
        "augmentation": bool(args.augment),
        "augment_strength": args.augment_strength,
        "dropout": args.dropout,
        "class_loss_weights": parse_class_loss_weights(args.class_loss_weights),
    }


def set_seed(seed: int) -> None:
    random.seed(seed)
    np.random.seed(seed)
    torch.manual_seed(seed)


def resolve_num_workers(requested: int) -> int:
    if requested >= 0:
        return requested
    # Auto mode: use all logical CPU cores for max data loading throughput.
    return os.cpu_count() or 8


def unwrap_model(model: nn.Module) -> nn.Module:
    if isinstance(model, nn.DataParallel):
        return model.module
    return model


def maybe_wrap_data_parallel(model: nn.Module, device: torch.device, enable: bool) -> nn.Module:
    if not enable or device.type != "cuda":
        return model
    gpu_count = torch.cuda.device_count()
    if gpu_count <= 1:
        return model
    print(f"[info] Enabling DataParallel on {gpu_count} GPUs")
    return nn.DataParallel(model)


def print_cuda_diagnostics() -> None:
    available = torch.cuda.is_available()
    print(f"[env] torch.cuda.is_available={available}")
    print(f"[env] torch_version={torch.__version__}, cuda_version={torch.version.cuda}")
    if not available:
        return
    device_idx = torch.cuda.current_device()
    device_name = torch.cuda.get_device_name(device_idx)
    print(f"[env] cuda.current_device={device_idx}")
    print(f"[env] cuda.device_name={device_name}")
    print(f"[env] cudnn.enabled={torch.backends.cudnn.enabled}")


def configure_torch_backend(device: torch.device) -> None:
    # Enable faster tensor-core math on modern NVIDIA GPUs.
    try:
        torch.set_float32_matmul_precision("high")
    except Exception:
        pass
    if device.type == "cuda":
        torch.backends.cuda.matmul.allow_tf32 = True
        torch.backends.cudnn.allow_tf32 = True


def json_print(payload: Dict[str, object]) -> None:
    print(json.dumps(payload, ensure_ascii=False))


def prepare_output_dir(output_dir: Path) -> Path:
    out = output_dir.resolve()
    out.mkdir(parents=True, exist_ok=True)
    return out


def count_csv_rows(csv_path: Path) -> int:
    with csv_path.open("r", encoding="utf-8") as f:
        return sum(1 for _ in f) - 1


def dataset_summary_line(samples: Sequence[Sample], val_ratio: float) -> str:
    unique_patients = len({s.patient_id for s in samples})
    return f"[info] Loaded samples={len(samples)}, unique_patients={unique_patients}, val_ratio={val_ratio}"


def split_samples_with_fallback(samples: Sequence[Sample], args: argparse.Namespace) -> Tuple[List[Sample], List[Sample]]:
    train_samples, val_samples = split_by_patient(samples, val_ratio=args.val_ratio, seed=args.seed)
    if not train_samples or not val_samples:
        if args.fallback_image_split:
            print("[warn] Patient-level split is empty; falling back to image-level random split.")
            train_samples, val_samples = split_by_image_random(samples, val_ratio=args.val_ratio, seed=args.seed)
        else:
            raise RuntimeError("Train/val split is empty; adjust val_ratio or dataset.")
    if not train_samples or not val_samples:
        raise RuntimeError("Train/val split is still empty after fallback; dataset too small.")
    return train_samples, val_samples


def resolve_nih_list_path(dataset_root: Path, filename: str) -> Path:
    direct_candidates = [
        dataset_root / "metadata" / filename,
        dataset_root / filename,
    ]
    for p in direct_candidates:
        if p.exists():
            return p
    recursive_candidates = sorted([p for p in dataset_root.rglob(filename) if p.is_file()])
    if recursive_candidates:
        return recursive_candidates[0]
    raise FileNotFoundError(
        f"{filename} not found. Expected under {dataset_root / 'metadata'} or dataset root."
    )


def load_name_list(path: Path) -> set[str]:
    return {x.strip() for x in path.read_text(encoding="utf-8").splitlines() if x.strip()}


def split_samples_nih_official(
    samples: Sequence[Sample], dataset_root: Path, args: argparse.Namespace
) -> Tuple[List[Sample], List[Sample], List[Sample], int]:
    train_val_path = resolve_nih_list_path(dataset_root, "train_val_list.txt")
    test_path = resolve_nih_list_path(dataset_root, "test_list.txt")
    train_val_names = load_name_list(train_val_path)
    test_names = load_name_list(test_path)

    train_val_pool: List[Sample] = []
    test_samples: List[Sample] = []
    dropped_unlisted = 0
    for sample in samples:
        name = sample.image_path.name
        if name in test_names:
            test_samples.append(sample)
        elif name in train_val_names:
            train_val_pool.append(sample)
        else:
            dropped_unlisted += 1

    if not train_val_pool:
        raise RuntimeError("NIH official split produced empty train/val pool; check train_val_list.txt.")
    if not test_samples:
        raise RuntimeError("NIH official split produced empty test set; check test_list.txt.")

    train_samples, val_samples = split_by_patient(train_val_pool, val_ratio=args.val_ratio, seed=args.seed)
    if not train_samples or not val_samples:
        if args.fallback_image_split:
            print("[warn] NIH train_val patient split is empty; falling back to image-level random split.")
            train_samples, val_samples = split_by_image_random(train_val_pool, val_ratio=args.val_ratio, seed=args.seed)
        else:
            raise RuntimeError("NIH train_val split is empty; adjust val_ratio or dataset.")
    if not train_samples or not val_samples:
        raise RuntimeError("NIH train/val split is still empty after fallback; dataset too small.")

    print(
        f"[info] NIH official lists: train_val={len(train_val_pool)}, test={len(test_samples)}, "
        f"dropped_unlisted={dropped_unlisted}"
    )
    return train_samples, val_samples, test_samples, dropped_unlisted


def save_summary(output_dir: Path, summary: Dict[str, object]) -> None:
    (output_dir / "nih_multitask_summary.json").write_text(
        json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8"
    )


def _get_metric(row: Dict[str, object], *keys: str, default: float = float("nan")) -> float:
    for key in keys:
        value = row.get(key)
        if value is not None:
            try:
                return float(value)
            except (TypeError, ValueError):
                return default
    return default


def build_training_report_markdown(summary: Dict[str, object]) -> str:
    history_raw = summary.get("history")
    history: List[Dict[str, object]] = history_raw if isinstance(history_raw, list) else []
    best_auc = float(summary.get("best_mean_auc", 0.0))
    best_epoch = int(float(summary.get("best_epoch", 0)))
    train_size = int(summary.get("train_size", 0))
    val_size = int(summary.get("val_size", 0))
    test_size = int(summary.get("test_size", 0))
    split_mode = str(summary.get("split_mode", "unknown"))
    best_checkpoint = str(summary.get("best_checkpoint", ""))
    last_checkpoint = str(summary.get("last_checkpoint", ""))

    final_train_loss = float("nan")
    final_val_auc = float("nan")
    overfit = False
    trend_note = "历史不足，无法判断趋势。"

    if history:
        last = history[-1]
        final_train_loss = _get_metric(last, "train_loss")
        final_val_auc = _get_metric(last, "val_mean_auc", "mean_auc")
        auc_series = [_get_metric(item, "val_mean_auc", "mean_auc") for item in history]
        if len(auc_series) >= 3 and auc_series[-1] < auc_series[-2] < auc_series[-3]:
            overfit = True
            trend_note = "验证AUC连续下降（最近3个epoch），存在明显过拟合风险。"
        elif final_val_auc < best_auc:
            trend_note = "验证AUC低于历史最佳，建议优先使用best checkpoint。"
        else:
            trend_note = "验证AUC稳定或改善。"

    class_items = [
        ("Pneumonia", "val_auc_Pneumonia", "auc_Pneumonia"),
        ("Nodule", "val_auc_Nodule", "auc_Nodule"),
        ("Mass", "val_auc_Mass", "auc_Mass"),
        ("Lung Opacity", "val_auc_Lung Opacity", "auc_Lung Opacity"),
    ]
    class_lines: List[str] = []
    for label, key1, key2 in class_items:
        values = [_get_metric(item, key1, key2) for item in history]
        values = [v for v in values if not np.isnan(v)]
        if values:
            class_lines.append(
                f"- {label}: best AUC={max(values):.4f}, final AUC={values[-1]:.4f}"
            )
        else:
            class_lines.append(f"- {label}: 无可用AUC")

    release_decision = "建议继续验证"
    if best_auc >= 0.75 and not overfit:
        release_decision = "可进入下一阶段验证（需结合阈值与混淆矩阵）"
    if overfit:
        release_decision = "暂不建议升级上线，优先处理过拟合"

    lines = [
        "# NIH 多任务训练报告",
        "",
        "## 1) 训练概览",
        f"- 数据划分: split_mode={split_mode}, train={train_size}, val={val_size}, test={test_size}",
        f"- 最佳轮次: epoch {best_epoch}",
        f"- 最佳验证AUC(mean): {best_auc:.4f}",
        f"- 末轮训练loss: {final_train_loss:.4f}" if not np.isnan(final_train_loss) else "- 末轮训练loss: N/A",
        f"- 末轮验证AUC(mean): {final_val_auc:.4f}" if not np.isnan(final_val_auc) else "- 末轮验证AUC(mean): N/A",
        "",
        "## 2) 趋势判断",
        f"- 结论: {trend_note}",
        "",
        "## 3) 各任务AUC",
        *class_lines,
        "",
        "## 4) 模型产物",
        f"- 推荐用于推理: `{best_checkpoint}`",
        f"- 最后一轮checkpoint: `{last_checkpoint}`",
        "",
        "## 5) 发布建议",
        f"- 结论: {release_decision}",
        "- 下一步: 结合 `evaluate_predictions.py` 生成阈值下的敏感度/特异度与混淆矩阵后再定版。",
        "",
    ]
    return "\n".join(lines)


def save_training_report(output_dir: Path, summary: Dict[str, object]) -> Path:
    report_text = build_training_report_markdown(summary)
    report_path = output_dir / "nih_training_report.md"
    report_path.write_text(report_text, encoding="utf-8")
    return report_path


def build_data_loaders(
    train_samples: Sequence[Sample],
    val_samples: Sequence[Sample],
    image_size: int,
    batch_size: int,
    num_workers: int,
    augment: bool,
    use_weighted_sampler: bool,
    sampler_power: float,
    sampler_max_weight: float,
    augment_strength: str,
    pin_memory: bool,
    prefetch_factor: int,
) -> Tuple[DataLoader, DataLoader]:
    train_transform = build_transforms(image_size=image_size, augment=augment, augment_strength=augment_strength)
    eval_transform = build_transforms(image_size=image_size, augment=False)
    train_ds = NihCxrDataset(train_samples, transform=train_transform)
    val_ds = NihCxrDataset(val_samples, transform=eval_transform)

    train_loader_kwargs: Dict[str, object] = {
        "batch_size": batch_size,
        "num_workers": num_workers,
        "pin_memory": pin_memory,
        "persistent_workers": num_workers > 0,
    }
    if num_workers > 0:
        train_loader_kwargs["prefetch_factor"] = prefetch_factor
    if use_weighted_sampler:
        sampler = build_weighted_sampler(
            train_samples, sampler_power=sampler_power, sampler_max_weight=sampler_max_weight
        )
        train_loader_kwargs["sampler"] = sampler
    else:
        train_loader_kwargs["shuffle"] = True

    train_loader = DataLoader(train_ds, **train_loader_kwargs)
    val_loader_kwargs: Dict[str, object] = {
        "batch_size": batch_size,
        "shuffle": False,
        "num_workers": num_workers,
        "pin_memory": pin_memory,
        "persistent_workers": num_workers > 0,
    }
    if num_workers > 0:
        val_loader_kwargs["prefetch_factor"] = prefetch_factor
    val_loader = DataLoader(val_ds, **val_loader_kwargs)
    return train_loader, val_loader


def build_eval_loader(
    samples: Sequence[Sample], image_size: int, batch_size: int, num_workers: int, pin_memory: bool
) -> DataLoader:
    ds = NihCxrDataset(samples, transform=build_transforms(image_size=image_size, augment=False))
    return DataLoader(
        ds,
        batch_size=batch_size,
        shuffle=False,
        num_workers=num_workers,
        pin_memory=pin_memory,
        persistent_workers=num_workers > 0,
    )


def build_transforms(image_size: int, augment: bool, augment_strength: str = "light") -> transforms.Compose:
    tfms: List[transforms.Compose] = [transforms.Resize((image_size, image_size))]
    if augment:
        if augment_strength == "strong":
            tfms.extend(
                [
                    transforms.RandomHorizontalFlip(p=0.5),
                    transforms.RandomAffine(degrees=12, translate=(0.05, 0.05), scale=(0.9, 1.1)),
                    transforms.ColorJitter(brightness=0.2, contrast=0.25),
                ]
            )
        else:
            tfms.extend(
                [
                    transforms.RandomHorizontalFlip(p=0.5),
                    transforms.RandomAffine(degrees=8, translate=(0.03, 0.03), scale=(0.95, 1.05)),
                    transforms.ColorJitter(brightness=0.15, contrast=0.2),
                ]
            )
    tfms.append(transforms.ToTensor())
    return transforms.Compose(tfms)


def build_weighted_sampler(
    samples: Sequence[Sample], sampler_power: float = 0.5, sampler_max_weight: float = 5.0
) -> WeightedRandomSampler:
    y = np.stack([s.target for s in samples], axis=0)
    pos = y.sum(axis=0)
    neg = y.shape[0] - pos
    class_weights = (neg + 1.0) / (pos + 1.0)
    class_weights = np.power(class_weights, sampler_power)
    sample_weights = 1.0 + (y * class_weights.reshape(1, -1)).sum(axis=1)
    sample_weights = np.clip(sample_weights, 1.0, sampler_max_weight)
    sample_weights = torch.tensor(sample_weights, dtype=torch.double)
    return WeightedRandomSampler(weights=sample_weights, num_samples=len(sample_weights), replacement=True)


def choose_device(cpu: bool) -> torch.device:
    return torch.device("cuda" if torch.cuda.is_available() and not cpu else "cpu")


def csv_sanity_check(csv_path: Path, min_csv_rows: int, allow_small_csv: bool) -> None:
    row_count = count_csv_rows(csv_path)
    if row_count < min_csv_rows:
        msg = (
            f"CSV rows={row_count} is suspiciously small. "
            "This usually indicates a sample metadata file, not full NIH labels. "
            "Please use full Data_Entry_2017(.csv or _v2020.csv), "
            "or pass --allow-small-csv to continue anyway."
        )
        if not allow_small_csv:
            raise RuntimeError(msg)
        print(f"[warn] {msg}")


def checkpoint_payload(model: nn.Module, args: argparse.Namespace, metrics: Dict[str, float]) -> Dict[str, object]:
    base_model = unwrap_model(model)
    payload = {
        "state_dict": base_model.state_dict(),
        "class_names": CLASS_NAMES,
        "image_size": args.image_size,
        "backbone": args.backbone,
        "dropout": args.dropout,
        "loss_type": args.loss_type,
        "class_loss_weights": parse_class_loss_weights(args.class_loss_weights),
        "metrics": metrics,
    }
    if args.teacher_checkpoint:
        payload["distillation"] = build_teacher_info(args)
    return payload


def build_lr_scheduler(
    optimizer: torch.optim.Optimizer, args: argparse.Namespace
) -> Optional[object]:
    if args.lr_scheduler == "none":
        return None
    if args.lr_scheduler == "cosine":
        return torch.optim.lr_scheduler.CosineAnnealingLR(optimizer, T_max=max(args.epochs, 1), eta_min=args.lr_min)
    return torch.optim.lr_scheduler.ReduceLROnPlateau(
        optimizer,
        mode="max",
        factor=args.lr_factor,
        patience=args.lr_patience,
        min_lr=args.lr_min,
    )


def ensure_min_samples(samples: Sequence[Sample]) -> None:
    if not samples:
        raise RuntimeError("No training samples found. Check dataset_root and CSV path.")
    if len(samples) < 2:
        raise RuntimeError(
            "Not enough samples to split train/val (need >=2). "
            "Check CSV/image matching and dataset completeness."
        )


def train_loop(
    model: nn.Module,
    teacher_model: Optional[nn.Module],
    train_loader: DataLoader,
    val_loader: DataLoader,
    args: argparse.Namespace,
    device: torch.device,
    best_path: Path,
    last_path: Path,
    epoch_dir: Path,
    train_samples: Sequence[Sample],
) -> Tuple[float, int, List[Dict[str, float]]]:
    torch.autograd.set_detect_anomaly(False)
    pos_weight = compute_pos_weight(train_samples).to(device)
    class_weights = torch.tensor(parse_class_loss_weights(args.class_loss_weights), dtype=torch.float32).to(device)
    if args.loss_type == "focal":
        criterion: nn.Module = FocalWithLogitsLoss(
            gamma=args.focal_gamma,
            alpha=args.focal_alpha,
            pos_weight=pos_weight,
            class_weights=class_weights,
        )
    else:
        criterion = BCEWithLogitsWeightedLoss(pos_weight=pos_weight, class_weights=class_weights)
    optimizer = torch.optim.AdamW(model.parameters(), lr=args.lr, weight_decay=args.weight_decay)
    scheduler = build_lr_scheduler(optimizer, args)
    amp_enabled = bool(args.amp and device.type == "cuda")
    scaler = build_grad_scaler(device_type=device.type, enabled=amp_enabled)

    best_auc = -1.0
    best_auc_for_patience = -1.0
    best_epoch = 0
    best_val_loss = float("inf")
    no_improve_epochs = 0
    val_loss_rise_streak = 0
    prev_val_loss = None
    history: List[Dict[str, float]] = []

    for epoch in range(1, args.epochs + 1):
        epoch_t0 = time.perf_counter()
        model.train()
        running_total = 0.0
        running_sup = 0.0
        running_kd = 0.0
        seen = 0

        pbar = tqdm(train_loader, desc=f"Epoch {epoch}/{args.epochs}", ncols=100)
        for x, y in pbar:
            x = x.to(device, non_blocking=True)
            y = y.to(device, non_blocking=True)
            optimizer.zero_grad(set_to_none=True)
            with amp_autocast(device_type=device.type, enabled=amp_enabled):
                student_logits = model(x)
                total_loss, sup_loss_value, kd_loss_value = train_step_loss(
                    student_logits=student_logits,
                    y=y,
                    criterion=criterion,
                    teacher_model=teacher_model,
                    x=x,
                    distill_alpha=args.distill_alpha,
                    distill_temp=args.distill_temp,
                )
            scaler.scale(total_loss).backward()
            if args.grad_clip > 0:
                scaler.unscale_(optimizer)
                nn.utils.clip_grad_norm_(model.parameters(), max_norm=args.grad_clip)
            scaler.step(optimizer)
            scaler.update()

            bs = x.shape[0]
            running_total += float(total_loss.item()) * bs
            running_sup += sup_loss_value * bs
            running_kd += kd_loss_value * bs
            seen += bs
            pbar.set_postfix(
                format_train_postfix(
                    supervised=running_sup / max(seen, 1),
                    distill=running_kd / max(seen, 1),
                    total=running_total / max(seen, 1),
                    use_distill=should_use_distillation(teacher_model, args.distill_alpha),
                )
            )

        metrics = evaluate(model, val_loader, device, prefix="val")
        metrics["epoch"] = float(epoch)
        metrics["train_loss"] = running_total / max(seen, 1)
        metrics["train_sup_loss"] = running_sup / max(seen, 1)
        epoch_seconds = max(time.perf_counter() - epoch_t0, 1e-6)
        metrics["epoch_seconds"] = epoch_seconds
        metrics["samples_per_sec"] = float(seen / epoch_seconds)
        if should_use_distillation(teacher_model, args.distill_alpha):
            metrics["train_kd_loss"] = running_kd / max(seen, 1)
        val_auc = float(metrics["val_mean_auc"])
        val_loss = float(metrics["val_loss"])
        if isinstance(scheduler, torch.optim.lr_scheduler.ReduceLROnPlateau):
            scheduler.step(val_auc)
        elif scheduler is not None:
            scheduler.step()
        metrics["lr"] = float(optimizer.param_groups[0]["lr"])
        history.append(metrics)
        json_print(metrics)

        ckpt = checkpoint_payload(model, args, metrics)
        torch.save(ckpt, last_path)
        if args.save_epoch_checkpoints:
            torch.save(ckpt, epoch_dir / f"epoch_{epoch}.pt")

        if val_loss < best_val_loss:
            best_val_loss = val_loss
        if prev_val_loss is not None and val_loss > prev_val_loss:
            val_loss_rise_streak += 1
        else:
            val_loss_rise_streak = 0
        prev_val_loss = val_loss

        if val_auc > best_auc:
            best_auc = val_auc
            best_epoch = epoch
            torch.save(ckpt, best_path)

        if val_auc > (best_auc_for_patience + args.early_stop_min_delta):
            best_auc_for_patience = val_auc
            no_improve_epochs = 0
        else:
            no_improve_epochs += 1

        if val_loss_rise_streak >= 2:
            print(
                "[warn] val_loss rose for >=2 consecutive epochs; risk of overfitting. "
                "Consider stronger augmentation or higher dropout."
            )
        if args.early_stop_patience > 0 and no_improve_epochs >= args.early_stop_patience:
            print(
                f"[info] Early stopping at epoch {epoch}: "
                f"val_mean_auc did not improve by >= {args.early_stop_min_delta} "
                f"for {args.early_stop_patience} epoch(s)."
            )
            break
    return best_auc, best_epoch, history


def compute_pos_weight(samples: Sequence[Sample]) -> torch.Tensor:
    y = np.stack([s.target for s in samples], axis=0)
    pos = y.sum(axis=0)
    neg = y.shape[0] - pos
    w = (neg + 1.0) / (pos + 1.0)
    return torch.tensor(w, dtype=torch.float32)


def binary_roc_auc_score(y_true: np.ndarray, y_score: np.ndarray) -> float:
    """Compute ROC AUC for binary labels without sklearn/scipy dependency."""
    y_true = np.asarray(y_true, dtype=np.int64)
    y_score = np.asarray(y_score, dtype=np.float64)
    pos = y_true == 1
    neg = y_true == 0
    n_pos = int(pos.sum())
    n_neg = int(neg.sum())
    if n_pos == 0 or n_neg == 0:
        raise ValueError("ROC AUC is undefined when only one class is present.")

    order = np.argsort(y_score, kind="mergesort")
    sorted_scores = y_score[order]
    ranks = np.empty_like(sorted_scores, dtype=np.float64)

    # Assign average ranks for ties (1-based ranks).
    i = 0
    n = sorted_scores.shape[0]
    while i < n:
        j = i + 1
        while j < n and sorted_scores[j] == sorted_scores[i]:
            j += 1
        avg_rank = 0.5 * (i + j - 1) + 1.0
        ranks[i:j] = avg_rank
        i = j

    ranks_original = np.empty_like(ranks)
    ranks_original[order] = ranks
    sum_pos_ranks = float(ranks_original[pos].sum())
    auc = (sum_pos_ranks - (n_pos * (n_pos + 1) / 2.0)) / (n_pos * n_neg)
    return float(auc)


def evaluate(model: nn.Module, loader: DataLoader, device: torch.device, prefix: str = "val") -> Dict[str, float]:
    model.eval()
    ys: List[np.ndarray] = []
    ps: List[np.ndarray] = []
    loss_sum = 0.0
    n = 0
    criterion = nn.BCEWithLogitsLoss()
    with torch.no_grad():
        for x, y in loader:
            x = x.to(device, non_blocking=True)
            y = y.to(device, non_blocking=True)
            logits = model(x)
            loss = criterion(logits, y)
            loss_sum += float(loss.item()) * x.shape[0]
            n += x.shape[0]
            ys.append(y.cpu().numpy())
            ps.append(torch.sigmoid(logits).cpu().numpy())

    y_true = np.concatenate(ys, axis=0)
    y_prob = np.concatenate(ps, axis=0)
    metrics: Dict[str, float] = {f"{prefix}_loss": loss_sum / max(n, 1)}
    aucs: List[float] = []
    for i, name in enumerate(CLASS_NAMES):
        try:
            auc = binary_roc_auc_score(y_true[:, i], y_prob[:, i])
        except ValueError:
            auc = float("nan")
        metrics[f"{prefix}_auc_{name}"] = auc
        if not np.isnan(auc):
            aucs.append(auc)
    metrics[f"{prefix}_mean_auc"] = float(np.mean(aucs)) if aucs else 0.0
    return metrics


def train(args: argparse.Namespace) -> None:
    validate_args(args)
    set_seed(args.seed)
    print_cuda_diagnostics()

    dataset_root = Path(args.dataset_root).resolve()
    csv_path = resolve_csv_path(dataset_root, args.csv_path)
    output_dir = prepare_output_dir(Path(args.output_dir))
    csv_sanity_check(csv_path, min_csv_rows=args.min_csv_rows, allow_small_csv=args.allow_small_csv)

    samples = load_samples(dataset_root, csv_path)
    ensure_min_samples(samples)
    print(dataset_summary_line(samples, args.val_ratio))
    test_samples: List[Sample] = []
    dropped_unlisted = 0
    if args.split_mode == "nih_official":
        train_samples, val_samples, test_samples, dropped_unlisted = split_samples_nih_official(
            samples=samples, dataset_root=dataset_root, args=args
        )
    else:
        train_samples, val_samples = split_samples_with_fallback(samples, args)

    device = choose_device(args.cpu)
    configure_torch_backend(device)
    pin_memory = bool(args.pin_memory and device.type == "cuda")
    num_workers = resolve_num_workers(args.num_workers)
    effective_batch_size = args.batch_size
    effective_lr = args.lr
    if args.auto_scale_batch_lr:
        scale = max(float(args.batch_size_scale), 1.0)
        effective_batch_size = max(1, int(round(args.batch_size * scale)))
        effective_lr = args.lr * scale
        print(
            f"[info] Auto scale enabled: batch_size {args.batch_size} -> {effective_batch_size}, "
            f"lr {args.lr:.6g} -> {effective_lr:.6g}"
        )
    args.batch_size = effective_batch_size
    args.lr = effective_lr
    if device.type == "cuda" and args.cudnn_benchmark:
        torch.backends.cudnn.benchmark = True
    gpu_count = torch.cuda.device_count() if device.type == "cuda" else 0
    print(
        f"[env] device={device.type}, num_workers={num_workers}, "
        f"batch_size={args.batch_size}, amp={args.amp}, prefetch_factor={args.prefetch_factor}, "
        f"pin_memory={pin_memory}, gpus={gpu_count}"
    )

    train_loader, val_loader = build_data_loaders(
        train_samples=train_samples,
        val_samples=val_samples,
        image_size=args.image_size,
        batch_size=args.batch_size,
        num_workers=num_workers,
        augment=args.augment,
        use_weighted_sampler=args.use_weighted_sampler,
        sampler_power=args.sampler_power,
        sampler_max_weight=args.sampler_max_weight,
        augment_strength=args.augment_strength,
        pin_memory=pin_memory,
        prefetch_factor=args.prefetch_factor,
    )
    model = build_model(args.backbone, dropout=args.dropout).to(device)
    model = maybe_wrap_data_parallel(model, device=device, enable=args.data_parallel)
    teacher_model = create_teacher_model(args, device=device)
    training_banner(args, train_size=len(train_samples), val_size=len(val_samples))

    best_path, last_path = build_checkpoint_paths(output_dir, args.backbone)
    epoch_dir = output_dir / "epochs"
    if args.save_epoch_checkpoints:
        epoch_dir.mkdir(parents=True, exist_ok=True)
    best_auc, best_epoch, history = train_loop(
        model=model,
        teacher_model=teacher_model,
        train_loader=train_loader,
        val_loader=val_loader,
        args=args,
        device=device,
        best_path=best_path,
        last_path=last_path,
        epoch_dir=epoch_dir,
        train_samples=train_samples,
    )

    test_metrics: Dict[str, float] = {}
    if test_samples:
        test_loader = build_eval_loader(
            samples=test_samples,
            image_size=args.image_size,
            batch_size=args.batch_size,
            num_workers=num_workers,
            pin_memory=pin_memory,
        )
        best_model = build_model(args.backbone, dropout=args.dropout).to(device)
        load_checkpoint_weights(best_model, best_path, device)
        test_metrics = evaluate(best_model, test_loader, device, prefix="test")
        json_print(test_metrics)

    summary = {
        "dataset_root": str(dataset_root),
        "csv_path": str(csv_path),
        "split_mode": args.split_mode,
        "dropped_unlisted": dropped_unlisted,
        "model": model_metadata(args),
        "distillation": build_teacher_info(args),
        "train_size": len(train_samples),
        "val_size": len(val_samples),
        "test_size": len(test_samples),
        "test_metrics": test_metrics,
        "best_mean_auc": best_auc,
        "best_epoch": best_epoch,
        "best_checkpoint": str(best_path),
        "last_checkpoint": str(last_path),
        "history": history,
    }
    save_summary(output_dir, summary)
    report_path = save_training_report(output_dir, summary)
    print(f"Training done. Best mean AUC={best_auc:.4f}")
    print(f"Best checkpoint: {best_path}")
    print(f"Training report: {report_path}")


def build_argparser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(description="Train NIH ChestXray14 multitask model for current AI service.")
    p.add_argument("--dataset-root", required=True, help="NIH dataset root containing images and Data_Entry_2017.csv")
    p.add_argument("--csv-path", default="", help="Optional path to Data_Entry_2017.csv")
    p.add_argument(
        "--split-mode",
        default="hash_patient",
        choices=SPLIT_MODE_CHOICES,
        help="Data split strategy: hash_patient or NIH official train_val/test lists",
    )
    p.add_argument("--output-dir", default="../models", help="Output dir for checkpoints")
    p.add_argument(
        "--backbone",
        default="densenet121",
        choices=BACKBONE_CHOICES,
        help="Student model backbone",
    )
    p.add_argument(
        "--teacher-backbone",
        default="",
        choices=[""] + BACKBONE_CHOICES,
        help="Teacher backbone for distillation (default: same as --backbone)",
    )
    p.add_argument("--teacher-checkpoint", default="", help="Path to teacher checkpoint (.pt)")
    p.add_argument("--distill-alpha", type=float, default=0.0, help="Distillation loss weight in [0,1]")
    p.add_argument("--distill-temp", type=float, default=2.0, help="Distillation temperature (>0)")
    p.add_argument("--loss-type", choices=["bce", "focal"], default="bce")
    p.add_argument("--focal-gamma", type=float, default=2.0)
    p.add_argument("--focal-alpha", type=float, default=0.25)
    p.add_argument("--class-loss-weights", default="1,1,1,1", help="Per-class loss weights, e.g. 1.6,1,1,1")
    p.add_argument("--augment", action="store_true", help="Enable training-time augmentation")
    p.add_argument("--augment-strength", choices=["light", "strong"], default="light")
    p.add_argument("--dropout", type=float, default=0.2, help="Classifier head dropout ratio")
    p.add_argument("--use-weighted-sampler", action="store_true", help="Use weighted random sampler for imbalance")
    p.add_argument("--sampler-power", type=float, default=0.5, help="Class-weight exponent for weighted sampler")
    p.add_argument("--sampler-max-weight", type=float, default=5.0, help="Max per-sample weight cap")
    p.add_argument("--amp", action="store_true", help="Enable mixed precision training on CUDA")
    p.add_argument("--grad-clip", type=float, default=1.0, help="Global grad norm clipping; <=0 disables")
    p.add_argument("--early-stop-patience", type=int, default=2, help="Stop if val_mean_auc plateaus (0 disables)")
    p.add_argument("--early-stop-min-delta", type=float, default=0.0, help="Min AUC improvement to reset early stop")
    p.add_argument("--image-size", type=int, default=224)
    p.add_argument("--batch-size", type=int, default=32)
    p.add_argument(
        "--auto-scale-batch-lr",
        action="store_true",
        help="Scale batch size and learning rate together for higher throughput.",
    )
    p.add_argument(
        "--batch-size-scale",
        type=float,
        default=2.0,
        help="Scale factor used when --auto-scale-batch-lr is enabled.",
    )
    p.add_argument("--epochs", type=int, default=8)
    p.add_argument("--lr", type=float, default=1e-4)
    p.add_argument("--lr-scheduler", choices=LR_SCHEDULER_CHOICES, default="plateau")
    p.add_argument("--lr-patience", type=int, default=1, help="ReduceLROnPlateau patience (epochs)")
    p.add_argument("--lr-factor", type=float, default=0.5, help="ReduceLROnPlateau factor")
    p.add_argument("--lr-min", type=float, default=1e-6, help="Minimum LR for schedulers")
    p.add_argument("--weight-decay", type=float, default=1e-4)
    p.add_argument("--val-ratio", type=float, default=0.15)
    p.add_argument("--num-workers", type=int, default=-1, help="-1 means auto-set to CPU core count")
    p.add_argument(
        "--pin-memory",
        dest="pin_memory",
        action="store_true",
        default=True,
        help="Pin host memory for faster H2D transfer on CUDA.",
    )
    p.add_argument(
        "--no-pin-memory",
        dest="pin_memory",
        action="store_false",
        help="Disable pinned memory in DataLoader.",
    )
    p.add_argument("--prefetch-factor", type=int, default=4)
    p.add_argument(
        "--data-parallel",
        action="store_true",
        help="Enable nn.DataParallel when multiple CUDA GPUs are available.",
    )
    p.add_argument(
        "--cudnn-benchmark",
        dest="cudnn_benchmark",
        action="store_true",
        default=True,
        help="Enable cudnn benchmark on CUDA",
    )
    p.add_argument(
        "--no-cudnn-benchmark",
        dest="cudnn_benchmark",
        action="store_false",
        help="Disable cudnn benchmark",
    )
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
    p.add_argument("--save-epoch-checkpoints", action="store_true", help="Save per-epoch checkpoints to output-dir/epochs")
    return p


if __name__ == "__main__":
    train(build_argparser().parse_args())


