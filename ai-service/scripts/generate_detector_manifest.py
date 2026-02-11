import argparse
import hashlib
import json
from pathlib import Path

import onnxruntime as ort


def sha256_file(path: Path) -> str:
    h = hashlib.sha256()
    with path.open("rb") as f:
        for chunk in iter(lambda: f.read(1024 * 1024), b""):
            h.update(chunk)
    return h.hexdigest().lower()


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate detector manifest/profile from ONNX model.")
    parser.add_argument("--model", required=True, help="Path to detector ONNX model")
    parser.add_argument("--profile-out", default="./models/detector_profile.json")
    parser.add_argument("--sha-out", default="./models/detector.sha256")
    parser.add_argument("--decoder-format", default="auto", choices=["auto", "yolo", "xyxy_cls"])
    parser.add_argument("--input-channels", type=int, default=1, choices=[1, 3])
    args = parser.parse_args()

    model_path = Path(args.model).resolve()
    if not model_path.exists():
        raise FileNotFoundError(f"Model not found: {model_path}")

    sess = ort.InferenceSession(str(model_path), providers=["CPUExecutionProvider"])
    input_meta = sess.get_inputs()[0]
    output_meta = sess.get_outputs()

    digest = sha256_file(model_path)
    sha_out = Path(args.sha_out).resolve()
    sha_out.parent.mkdir(parents=True, exist_ok=True)
    sha_out.write_text(digest, encoding="utf-8")

    profile = {
        "model_path": str(model_path),
        "sha256": digest,
        "decoder_format": args.decoder_format,
        "input_channels": args.input_channels,
        "input_scale": 255.0,
        "input_mean": [0.0] if args.input_channels == 1 else [0.0, 0.0, 0.0],
        "input_std": [1.0] if args.input_channels == 1 else [1.0, 1.0, 1.0],
        "class_map": {
            "0": "结节(Nodule)",
            "1": "肿块(Mass)",
            "2": "浸润/实变(Opacity)",
            "3": "肺炎(Pneumonia)",
        },
        "io": {
            "input_name": input_meta.name,
            "input_shape": [str(v) for v in input_meta.shape],
            "output_names": [o.name for o in output_meta],
            "output_shapes": [[str(v) for v in o.shape] for o in output_meta],
        },
    }

    profile_out = Path(args.profile_out).resolve()
    profile_out.parent.mkdir(parents=True, exist_ok=True)
    profile_out.write_text(json.dumps(profile, ensure_ascii=False, indent=2), encoding="utf-8")

    print(f"Model: {model_path}")
    print(f"SHA256: {digest}")
    print(f"Wrote profile: {profile_out}")
    print(f"Wrote sha256: {sha_out}")


if __name__ == "__main__":
    main()
