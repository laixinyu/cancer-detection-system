import argparse
from pathlib import Path
from typing import List

import numpy as np
import onnxruntime as ort


def describe_shape(shape: List[object]) -> str:
    parts = []
    for v in shape:
        if isinstance(v, str):
            parts.append(v)
        elif v is None:
            parts.append("?")
        else:
            parts.append(str(v))
    return "[" + ", ".join(parts) + "]"


def main() -> None:
    parser = argparse.ArgumentParser(description="Check detector ONNX output compatibility.")
    parser.add_argument("--model", default="../models/cxr_detector.onnx")
    parser.add_argument("--size", type=int, default=640, help="Input size (square)")
    args = parser.parse_args()

    script_dir = Path(__file__).resolve().parent
    model_path = (script_dir / args.model).resolve()
    if not model_path.exists():
        raise FileNotFoundError(f"Detector model not found: {model_path}")

    sess = ort.InferenceSession(str(model_path), providers=["CPUExecutionProvider"])
    input_meta = sess.get_inputs()[0]
    output_meta = sess.get_outputs()

    print(f"Model: {model_path}")
    print(f"Input name: {input_meta.name}")
    print(f"Input shape: {describe_shape(list(input_meta.shape))}")
    print("Outputs:")
    for i, out in enumerate(output_meta):
        print(f"  - [{i}] {out.name} shape={describe_shape(list(out.shape))}")

    x = np.random.rand(1, 1, args.size, args.size).astype(np.float32)
    outputs = sess.run(None, {input_meta.name: x})
    if not outputs:
        raise RuntimeError("Model returned no outputs")

    primary = np.array(outputs[0])
    print(f"\nPrimary output runtime shape: {primary.shape}")

    # Compatibility hints for current decoder implementation.
    hint = "UNKNOWN"
    if primary.ndim == 3 and primary.shape[0] == 1:
        # e.g. [1, N, C] or [1, C, N]
        c1, c2 = primary.shape[1], primary.shape[2]
        if c2 >= 6:
            hint = "LIKELY [1, N, C] (xyxy+score+cls or cxcywh+probs)"
        elif c1 >= 6:
            hint = "LIKELY [1, C, N] (will be transposed)"
    elif primary.ndim == 2:
        if primary.shape[1] >= 6 or primary.shape[0] >= 6:
            hint = "LIKELY [N, C] or [C, N]"

    print(f"Decoder compatibility hint: {hint}")
    if hint == "UNKNOWN":
        print("WARNING: Output layout may be incompatible with current decoder in ai-service/app/main.py")
    else:
        print("OK: Output layout appears compatible with current decoder logic.")


if __name__ == "__main__":
    main()
