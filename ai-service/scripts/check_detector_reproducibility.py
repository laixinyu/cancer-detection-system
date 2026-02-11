import argparse
import json
from pathlib import Path
from typing import List, Tuple

import cv2
import numpy as np
import onnxruntime as ort
from PIL import Image


def load_gray(path: Path, size: int) -> np.ndarray:
    image = Image.open(path).convert("L")
    arr = np.array(image)
    resized = cv2.resize(arr, (size, size))
    return resized.astype(np.float32) / 255.0


def decode_simple(output: np.ndarray, conf_thr: float = 0.25) -> np.ndarray:
    out = np.array(output)
    if out.ndim == 3 and out.shape[0] == 1:
        out = out[0]
    if out.ndim == 2 and out.shape[0] < out.shape[1] and out.shape[0] >= 6:
        out = out.T
    if out.ndim != 2:
        return np.zeros((0, 6), dtype=np.float32)

    rows: List[List[float]] = []
    for row in out:
        if row.shape[0] < 6:
            continue
        if row.shape[0] <= 8:
            x1, y1, x2, y2, score, cls = row[:6]
            conf = float(score)
            class_id = float(cls)
        else:
            cx, cy, w, h = row[:4]
            probs = row[4:]
            class_id = float(np.argmax(probs))
            conf = float(probs[int(class_id)])
            x1 = cx - w / 2
            y1 = cy - h / 2
            x2 = cx + w / 2
            y2 = cy + h / 2
        if conf < conf_thr:
            continue
        rows.append([float(x1), float(y1), float(x2), float(y2), conf, class_id])
    if not rows:
        return np.zeros((0, 6), dtype=np.float32)
    arr = np.array(rows, dtype=np.float32)
    arr = arr[np.argsort(-arr[:, 4])]
    return arr[:20]


def compare_runs(ref: np.ndarray, cur: np.ndarray) -> Tuple[float, float]:
    if ref.shape != cur.shape:
        return float("inf"), float("inf")
    if ref.size == 0:
        return 0.0, 0.0
    score_delta = float(np.max(np.abs(ref[:, 4] - cur[:, 4])))
    box_delta = float(np.max(np.abs(ref[:, :4] - cur[:, :4])))
    return score_delta, box_delta


def main() -> None:
    parser = argparse.ArgumentParser(description="Check detector inference reproducibility.")
    parser.add_argument("--model", required=True, help="Detector ONNX model path")
    parser.add_argument("--image", required=True, help="Test image path")
    parser.add_argument("--runs", type=int, default=20)
    parser.add_argument("--size", type=int, default=640)
    parser.add_argument("--score-eps", type=float, default=1e-6)
    parser.add_argument("--box-eps", type=float, default=1e-4)
    parser.add_argument("--out", default="", help="Optional json report path")
    args = parser.parse_args()

    model_path = Path(args.model).resolve()
    image_path = Path(args.image).resolve()
    if not model_path.exists():
        raise FileNotFoundError(f"Model not found: {model_path}")
    if not image_path.exists():
        raise FileNotFoundError(f"Image not found: {image_path}")

    so = ort.SessionOptions()
    so.intra_op_num_threads = 1
    so.inter_op_num_threads = 1
    sess = ort.InferenceSession(str(model_path), sess_options=so, providers=["CPUExecutionProvider"])
    input_name = sess.get_inputs()[0].name

    x = load_gray(image_path, args.size)
    input_tensor = x[np.newaxis, np.newaxis, :, :].astype(np.float32)

    runs = []
    for _ in range(args.runs):
        out = sess.run(None, {input_name: input_tensor})[0]
        decoded = decode_simple(out)
        runs.append(decoded)

    ref = runs[0]
    score_deltas: List[float] = []
    box_deltas: List[float] = []
    for cur in runs[1:]:
        sd, bd = compare_runs(ref, cur)
        score_deltas.append(sd)
        box_deltas.append(bd)

    max_score_delta = max(score_deltas) if score_deltas else 0.0
    max_box_delta = max(box_deltas) if box_deltas else 0.0
    passed = max_score_delta <= args.score_eps and max_box_delta <= args.box_eps

    report = {
        "model": str(model_path),
        "image": str(image_path),
        "runs": args.runs,
        "max_score_delta": max_score_delta,
        "max_box_delta": max_box_delta,
        "score_eps": args.score_eps,
        "box_eps": args.box_eps,
        "passed": passed,
        "reference_shape": list(ref.shape),
    }
    print(json.dumps(report, indent=2, ensure_ascii=False))

    if args.out:
        out_path = Path(args.out).resolve()
        out_path.parent.mkdir(parents=True, exist_ok=True)
        out_path.write_text(json.dumps(report, indent=2, ensure_ascii=False), encoding="utf-8")

    if not passed:
        raise SystemExit(2)


if __name__ == "__main__":
    main()
