import argparse
from pathlib import Path

import torch
from torch import nn
from torchvision import models


class_names_expected = ["Pneumonia", "Nodule", "Mass", "Lung Opacity"]


def build_model() -> nn.Module:
    model = models.densenet121(weights=None)
    old_conv = model.features.conv0
    new_conv = nn.Conv2d(
        1,
        old_conv.out_channels,
        kernel_size=old_conv.kernel_size,
        stride=old_conv.stride,
        padding=old_conv.padding,
        bias=old_conv.bias is not None,
    )
    model.features.conv0 = new_conv
    in_features = model.classifier.in_features
    model.classifier = nn.Linear(in_features, 4)
    return model


def export_onnx(checkpoint_path: Path, output_path: Path, input_size: int, opset: int) -> None:
    ckpt = torch.load(checkpoint_path, map_location="cpu")
    class_names = ckpt.get("class_names")
    if class_names and list(class_names) != class_names_expected:
        raise RuntimeError(f"Unexpected class order in checkpoint: {class_names}")

    model = build_model()
    model.load_state_dict(ckpt["state_dict"], strict=True)
    model.eval()

    dummy = torch.randn(1, 1, input_size, input_size, dtype=torch.float32)
    output_path.parent.mkdir(parents=True, exist_ok=True)
    torch.onnx.export(
        model,
        dummy,
        str(output_path),
        export_params=True,
        opset_version=opset,
        do_constant_folding=True,
        input_names=["input"],
        output_names=["task_logits"],
        dynamic_axes={"input": {0: "batch"}, "task_logits": {0: "batch"}},
    )
    print(f"Exported ONNX to: {output_path}")
    print(f"Output order: {class_names_expected}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Export NIH-trained multitask checkpoint to ONNX.")
    parser.add_argument("--checkpoint", required=True, help="Path to .pt checkpoint")
    parser.add_argument("--output", default="../models/cxr_multitask.onnx")
    parser.add_argument("--input-size", type=int, default=224)
    parser.add_argument("--opset", type=int, default=17)
    args = parser.parse_args()

    script_dir = Path(__file__).resolve().parent
    checkpoint_path = Path(args.checkpoint).resolve()
    output_path = (script_dir / args.output).resolve()
    export_onnx(checkpoint_path, output_path, args.input_size, args.opset)


if __name__ == "__main__":
    main()
