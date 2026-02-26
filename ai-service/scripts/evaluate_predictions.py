import argparse
import csv
import json
from pathlib import Path
from typing import Dict, List, Tuple

import numpy as np


def load_csv(path: Path, task: str = "") -> Tuple[np.ndarray, np.ndarray]:
    y_true: List[int] = []
    y_score: List[float] = []
    task_filter = task.strip().lower()
    with path.open("r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        if "y_true" not in reader.fieldnames or "y_score" not in reader.fieldnames:
            raise ValueError("CSV must include columns: y_true,y_score")
        for row in reader:
            if task_filter:
                row_task = str(row.get("task", "")).strip().lower()
                if row_task != task_filter:
                    continue
            y_true.append(int(row["y_true"]))
            y_score.append(float(row["y_score"]))
    if not y_true:
        msg = f"No rows matched task={task_filter}" if task_filter else "No rows found in CSV"
        raise ValueError(msg)
    return np.array(y_true, dtype=np.int32), np.array(y_score, dtype=np.float64)


def confusion(y_true: np.ndarray, y_pred: np.ndarray) -> Dict[str, int]:
    tp = int(np.sum((y_true == 1) & (y_pred == 1)))
    tn = int(np.sum((y_true == 0) & (y_pred == 0)))
    fp = int(np.sum((y_true == 0) & (y_pred == 1)))
    fn = int(np.sum((y_true == 1) & (y_pred == 0)))
    return {"tp": tp, "tn": tn, "fp": fp, "fn": fn}


def safe_div(num: float, den: float) -> float:
    return float(num / den) if den > 0 else 0.0


def metrics_from_conf(cm: Dict[str, int]) -> Dict[str, float]:
    tp, tn, fp, fn = cm["tp"], cm["tn"], cm["fp"], cm["fn"]
    sensitivity = safe_div(tp, tp + fn)
    specificity = safe_div(tn, tn + fp)
    ppv = safe_div(tp, tp + fp)
    npv = safe_div(tn, tn + fn)
    accuracy = safe_div(tp + tn, tp + tn + fp + fn)
    return {
        "sensitivity": sensitivity,
        "specificity": specificity,
        "ppv": ppv,
        "npv": npv,
        "accuracy": accuracy,
    }


def auroc(y_true: np.ndarray, y_score: np.ndarray) -> float:
    pos = y_true == 1
    neg = y_true == 0
    n_pos = int(np.sum(pos))
    n_neg = int(np.sum(neg))
    if n_pos == 0 or n_neg == 0:
        return 0.0

    order = np.argsort(y_score)
    ranks = np.empty_like(order, dtype=np.float64)
    ranks[order] = np.arange(1, len(y_score) + 1)
    sum_ranks_pos = float(np.sum(ranks[pos]))
    auc = (sum_ranks_pos - n_pos * (n_pos + 1) / 2.0) / (n_pos * n_neg)
    return float(np.clip(auc, 0.0, 1.0))


def ece(y_true: np.ndarray, y_score: np.ndarray, bins: int = 10) -> float:
    y_score = np.clip(y_score, 0.0, 1.0)
    edges = np.linspace(0.0, 1.0, bins + 1)
    total = len(y_score)
    result = 0.0
    for i in range(bins):
        left, right = edges[i], edges[i + 1]
        mask = (y_score >= left) & (y_score < right if i < bins - 1 else y_score <= right)
        if not np.any(mask):
            continue
        conf = float(np.mean(y_score[mask]))
        acc = float(np.mean(y_true[mask]))
        result += (np.sum(mask) / total) * abs(acc - conf)
    return float(result)


def evaluate(y_true: np.ndarray, y_score: np.ndarray, thr_sens: float, thr_spec: float) -> Dict[str, object]:
    y_pred_sens = (y_score >= thr_sens).astype(np.int32)
    y_pred_spec = (y_score >= thr_spec).astype(np.int32)

    cm_sens = confusion(y_true, y_pred_sens)
    cm_spec = confusion(y_true, y_pred_spec)

    return {
        "n_samples": int(len(y_true)),
        "prevalence": float(np.mean(y_true)),
        "auroc": auroc(y_true, y_score),
        "ece_10bins": ece(y_true, y_score, bins=10),
        "high_sensitivity_operating_point": {
            "threshold": thr_sens,
            "confusion": cm_sens,
            "metrics": metrics_from_conf(cm_sens),
        },
        "high_specificity_operating_point": {
            "threshold": thr_spec,
            "confusion": cm_spec,
            "metrics": metrics_from_conf(cm_spec),
        },
    }


def main() -> None:
    parser = argparse.ArgumentParser(description="Evaluate model predictions from CSV file.")
    parser.add_argument("--csv", required=True, help="CSV with columns y_true,y_score")
    parser.add_argument("--task", default="", help="Optional task filter (requires CSV column 'task')")
    parser.add_argument("--thr-sens", type=float, default=0.30, help="High-sensitivity threshold")
    parser.add_argument("--thr-spec", type=float, default=0.70, help="High-specificity threshold")
    parser.add_argument("--out", default="", help="Optional JSON output file path")
    args = parser.parse_args()

    y_true, y_score = load_csv(Path(args.csv), task=args.task)
    report = evaluate(y_true, y_score, args.thr_sens, args.thr_spec)
    print(json.dumps(report, ensure_ascii=False, indent=2))

    if args.out:
        out_path = Path(args.out)
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")


if __name__ == "__main__":
    main()
