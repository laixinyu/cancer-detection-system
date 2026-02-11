import argparse
import csv
import json
from dataclasses import dataclass
from pathlib import Path
from typing import Dict, List, Tuple

import numpy as np


TASKS = {"pneumonia", "lesion"}


@dataclass
class Metrics:
    sensitivity: float
    specificity: float
    ppv: float
    npv: float
    accuracy: float


def safe_div(a: float, b: float) -> float:
    return float(a / b) if b > 0 else 0.0


def compute_metrics(y_true: np.ndarray, y_score: np.ndarray, threshold: float) -> Metrics:
    y_pred = (y_score >= threshold).astype(np.int32)
    tp = int(np.sum((y_true == 1) & (y_pred == 1)))
    tn = int(np.sum((y_true == 0) & (y_pred == 0)))
    fp = int(np.sum((y_true == 0) & (y_pred == 1)))
    fn = int(np.sum((y_true == 1) & (y_pred == 0)))
    return Metrics(
        sensitivity=safe_div(tp, tp + fn),
        specificity=safe_div(tn, tn + fp),
        ppv=safe_div(tp, tp + fp),
        npv=safe_div(tn, tn + fn),
        accuracy=safe_div(tp + tn, tp + tn + fp + fn),
    )


def fit_temperature(logits: np.ndarray, y_true: np.ndarray) -> float:
    logits = logits.astype(np.float64)
    y = y_true.astype(np.float64)
    best_t = 1.0
    best_nll = float("inf")
    for t in np.arange(0.5, 3.01, 0.01):
        p = 1.0 / (1.0 + np.exp(-logits / t))
        p = np.clip(p, 1e-8, 1 - 1e-8)
        nll = float(-np.mean(y * np.log(p) + (1.0 - y) * np.log(1.0 - p)))
        if nll < best_nll:
            best_nll = nll
            best_t = float(t)
    return best_t


def pick_threshold_for_target_sensitivity(
    y_true: np.ndarray, y_score: np.ndarray, target_sens: float
) -> Tuple[float, Metrics]:
    candidates = np.unique(y_score)
    candidates = np.concatenate(([0.0], candidates, [1.0]))
    best_thr = 0.5
    best_metrics = compute_metrics(y_true, y_score, best_thr)

    feasible: List[Tuple[float, Metrics]] = []
    for thr in candidates:
        m = compute_metrics(y_true, y_score, float(thr))
        if m.sensitivity >= target_sens:
            feasible.append((float(thr), m))

    if feasible:
        # maximize specificity first, then pick highest threshold for stability
        feasible.sort(key=lambda x: (x[1].specificity, x[0]), reverse=True)
        best_thr, best_metrics = feasible[0]
    return best_thr, best_metrics


def pick_threshold_for_target_specificity(
    y_true: np.ndarray, y_score: np.ndarray, target_spec: float
) -> Tuple[float, Metrics]:
    candidates = np.unique(y_score)
    candidates = np.concatenate(([0.0], candidates, [1.0]))
    best_thr = 0.5
    best_metrics = compute_metrics(y_true, y_score, best_thr)

    feasible: List[Tuple[float, Metrics]] = []
    for thr in candidates:
        m = compute_metrics(y_true, y_score, float(thr))
        if m.specificity >= target_spec:
            feasible.append((float(thr), m))

    if feasible:
        # maximize sensitivity first, then pick lowest threshold for recall-friendly behavior
        feasible.sort(key=lambda x: (x[1].sensitivity, -x[0]), reverse=True)
        best_thr, best_metrics = feasible[0]
    return best_thr, best_metrics


def load_rows(path: Path) -> Dict[str, Dict[str, np.ndarray]]:
    buckets: Dict[str, Dict[str, List[float]]] = {
        "pneumonia": {"y_true": [], "y_score": [], "y_logit": []},
        "lesion": {"y_true": [], "y_score": [], "y_logit": []},
    }

    with path.open("r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        required = {"task", "y_true", "y_score"}
        if not required.issubset(set(reader.fieldnames or [])):
            raise ValueError("CSV must contain columns: task,y_true,y_score")

        for row in reader:
            task = (row.get("task") or "").strip().lower()
            if task not in TASKS:
                continue
            buckets[task]["y_true"].append(float(row["y_true"]))
            buckets[task]["y_score"].append(float(row["y_score"]))
            if row.get("y_logit") not in (None, ""):
                buckets[task]["y_logit"].append(float(row["y_logit"]))

    result: Dict[str, Dict[str, np.ndarray]] = {}
    for task, values in buckets.items():
        y_true = np.array(values["y_true"], dtype=np.int32)
        y_score = np.array(values["y_score"], dtype=np.float64)
        y_logit = np.array(values["y_logit"], dtype=np.float64)
        if y_true.size == 0:
            raise ValueError(f"No rows found for task={task}")
        result[task] = {"y_true": y_true, "y_score": y_score, "y_logit": y_logit}
    return result


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Derive clinically-oriented calibration and thresholds from validation data."
    )
    parser.add_argument("--csv", required=True, help="CSV with task,y_true,y_score[,y_logit]")
    parser.add_argument("--target-sens", type=float, default=0.95, help="Target sensitivity for screening point")
    parser.add_argument("--target-spec", type=float, default=0.90, help="Target specificity for confirmatory point")
    parser.add_argument("--out", default="./models/clinical_config.json", help="Output JSON path")
    args = parser.parse_args()

    data = load_rows(Path(args.csv))
    tasks_payload: Dict[str, object] = {}
    temperatures: List[float] = []

    for task, values in data.items():
        y_true = values["y_true"]
        y_score = np.clip(values["y_score"], 1e-6, 1 - 1e-6)
        if values["y_logit"].size == y_true.size:
            logits = values["y_logit"]
        else:
            logits = np.log(y_score / (1.0 - y_score))

        temp = fit_temperature(logits, y_true)
        temperatures.append(temp)
        calibrated = 1.0 / (1.0 + np.exp(-logits / temp))

        hs_thr, hs_metrics = pick_threshold_for_target_sensitivity(y_true, calibrated, args.target_sens)
        hp_thr, hp_metrics = pick_threshold_for_target_specificity(y_true, calibrated, args.target_spec)

        tasks_payload[task] = {
            "high_sensitivity_threshold": float(hs_thr),
            "high_specificity_threshold": float(hp_thr),
            "achieved_high_sensitivity_metrics": hs_metrics.__dict__,
            "achieved_high_specificity_metrics": hp_metrics.__dict__,
            "n_samples": int(y_true.size),
        }

    output = {
        "version": "clinical-calibration-v1",
        "global": {
            "temperature": float(np.mean(temperatures)),
            "target_sensitivity": float(args.target_sens),
            "target_specificity": float(args.target_spec),
        },
        "tasks": tasks_payload,
    }

    out_path = Path(args.out)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_path.write_text(json.dumps(output, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(output, ensure_ascii=False, indent=2))
    print(f"\nSaved clinical config: {out_path.resolve()}")


if __name__ == "__main__":
    main()
