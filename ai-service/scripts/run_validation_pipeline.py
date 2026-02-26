import argparse
import csv
import json
import subprocess
import sys
from pathlib import Path
from typing import Dict, List, Tuple

import numpy as np

from evaluate_predictions import evaluate


SUPPORTED_TASKS = ("pneumonia", "lesion")


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(
        description="Run validation pipeline: export predictions -> evaluate -> derive clinical config."
    )
    p.add_argument("--dataset-root", required=True, help="Dataset root for split reproduction")
    p.add_argument("--csv-path", default="", help="Optional metadata CSV path")
    p.add_argument("--checkpoint", required=True, help="Checkpoint used for export inference")
    p.add_argument("--split-mode", choices=["hash_patient", "nih_official"], default="nih_official")
    p.add_argument("--target-split", choices=["val", "test", "both"], default="val")
    p.add_argument("--predictions-csv", default="", help="Use existing predictions CSV and skip export")
    p.add_argument("--output-dir", default="ai-service/models/eval", help="Output directory")
    p.add_argument("--batch-size", type=int, default=64)
    p.add_argument("--num-workers", type=int, default=-1)
    p.add_argument("--cpu", action="store_true")
    p.add_argument("--thr-sens", type=float, default=0.30)
    p.add_argument("--thr-spec", type=float, default=0.70)
    p.add_argument("--target-sens", type=float, default=0.95)
    p.add_argument("--target-spec", type=float, default=0.90)
    p.add_argument(
        "--clinical-config-out",
        default="ai-service/models/clinical_config.json",
        help="Path for generated clinical config JSON",
    )
    p.add_argument("--min-csv-rows", type=int, default=1000)
    p.add_argument("--allow-small-csv", action="store_true")
    return p.parse_args()


def run_cmd(cmd: List[str]) -> None:
    print(f"[pipeline] $ {' '.join(cmd)}")
    subprocess.run(cmd, check=True)


def load_predictions(path: Path) -> Dict[Tuple[str, str], Tuple[np.ndarray, np.ndarray]]:
    buckets: Dict[Tuple[str, str], Dict[str, List[float]]] = {}
    with path.open("r", encoding="utf-8") as f:
        reader = csv.DictReader(f)
        required = {"split", "task", "y_true", "y_score"}
        missing = required.difference(set(reader.fieldnames or []))
        if missing:
            raise ValueError(f"Predictions CSV missing required columns: {sorted(missing)}")
        for row in reader:
            split = str(row.get("split", "")).strip().lower()
            task = str(row.get("task", "")).strip().lower()
            if not split or task not in SUPPORTED_TASKS:
                continue
            key = (split, task)
            if key not in buckets:
                buckets[key] = {"y_true": [], "y_score": []}
            buckets[key]["y_true"].append(float(row["y_true"]))
            buckets[key]["y_score"].append(float(row["y_score"]))

    data: Dict[Tuple[str, str], Tuple[np.ndarray, np.ndarray]] = {}
    for key, values in buckets.items():
        y_true = np.array(values["y_true"], dtype=np.int32)
        y_score = np.array(values["y_score"], dtype=np.float64)
        if y_true.size == 0:
            continue
        data[key] = (y_true, y_score)
    return data


def evaluate_all(
    predictions_csv: Path,
    output_dir: Path,
    thr_sens: float,
    thr_spec: float,
) -> Dict[str, str]:
    eval_paths: Dict[str, str] = {}
    grouped = load_predictions(predictions_csv)
    if not grouped:
        raise RuntimeError("No supported task rows found in predictions CSV.")

    for (split, task), (y_true, y_score) in grouped.items():
        report = evaluate(y_true, y_score, thr_sens=thr_sens, thr_spec=thr_spec)
        out_name = f"eval_{split}_{task}.json"
        out_path = output_dir / out_name
        out_path.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
        eval_paths[f"{split}:{task}"] = str(out_path)
        print(f"[pipeline] wrote {out_path}")

    return eval_paths


def main() -> None:
    args = parse_args()
    output_dir = Path(args.output_dir).expanduser().resolve()
    output_dir.mkdir(parents=True, exist_ok=True)

    predictions_csv = Path(args.predictions_csv).expanduser().resolve() if args.predictions_csv else output_dir / "predictions.csv"
    if not args.predictions_csv:
        exporter = Path(__file__).with_name("export_validation_predictions.py")
        cmd = [
            sys.executable,
            str(exporter),
            "--dataset-root",
            args.dataset_root,
            "--checkpoint",
            args.checkpoint,
            "--split-mode",
            args.split_mode,
            "--target-split",
            args.target_split,
            "--batch-size",
            str(args.batch_size),
            "--num-workers",
            str(args.num_workers),
            "--min-csv-rows",
            str(args.min_csv_rows),
            "--out",
            str(predictions_csv),
        ]
        if args.csv_path:
            cmd.extend(["--csv-path", args.csv_path])
        if args.cpu:
            cmd.append("--cpu")
        if args.allow_small_csv:
            cmd.append("--allow-small-csv")
        run_cmd(cmd)

    eval_paths = evaluate_all(
        predictions_csv=predictions_csv,
        output_dir=output_dir,
        thr_sens=args.thr_sens,
        thr_spec=args.thr_spec,
    )

    derive_script = Path(__file__).with_name("derive_clinical_config.py")
    clinical_out = Path(args.clinical_config_out).expanduser().resolve()
    cmd = [
        sys.executable,
        str(derive_script),
        "--csv",
        str(predictions_csv),
        "--target-sens",
        str(args.target_sens),
        "--target-spec",
        str(args.target_spec),
        "--out",
        str(clinical_out),
    ]
    run_cmd(cmd)

    summary = {
        "predictions_csv": str(predictions_csv),
        "eval_reports": eval_paths,
        "clinical_config": str(clinical_out),
        "thresholds": {
            "evaluation_high_sensitivity": args.thr_sens,
            "evaluation_high_specificity": args.thr_spec,
            "target_sensitivity": args.target_sens,
            "target_specificity": args.target_spec,
        },
    }
    summary_path = output_dir / "pipeline_summary.json"
    summary_path.write_text(json.dumps(summary, ensure_ascii=False, indent=2), encoding="utf-8")
    print(json.dumps(summary, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
