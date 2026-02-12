import argparse
from pathlib import Path

import torch
from torch import nn
from torchvision import models


class_names_expected = ["Pneumonia", "Nodule", "Mass", "Lung Opacity"]


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
    return new_conv


def build_model(backbone: str, dropout: float = 0.0) -> nn.Module:
    if backbone == "densenet121":
        model = models.densenet121(weights=None)
        model.features.conv0 = adapt_conv2d_in_channels(model.features.conv0, in_channels=1)
        in_features = model.classifier.in_features
        head: nn.Module = nn.Linear(in_features, 4)
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier = head
        return model

    if backbone == "efficientnet_b0":
        model = models.efficientnet_b0(weights=None)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        head: nn.Module = nn.Linear(in_features, 4)
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier[-1] = head
        return model

    if backbone == "efficientnet_v2_s":
        model = models.efficientnet_v2_s(weights=None)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        head: nn.Module = nn.Linear(in_features, 4)
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier[-1] = head
        return model

    if backbone == "mobilenet_v3_small":
        model = models.mobilenet_v3_small(weights=None)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        head: nn.Module = nn.Linear(in_features, 4)
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier[-1] = head
        return model

    if backbone == "convnext_tiny":
        model = models.convnext_tiny(weights=None)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        head: nn.Module = nn.Linear(in_features, 4)
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.classifier[-1] = head
        return model

    if backbone == "convnext_large":
        model = models.convnext_large(weights=None)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.classifier[-1].in_features
        model.classifier[-1] = nn.Linear(in_features, 4)
        return model

    if backbone == "swin_b":
        model = models.swin_b(weights=None)
        model.features[0][0] = adapt_conv2d_in_channels(model.features[0][0], in_channels=1)
        in_features = model.head.in_features
        head: nn.Module = nn.Linear(in_features, 4)
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.head = head
        return model

    if backbone == "vit_b_16":
        model = models.vit_b_16(weights=None)
        model.conv_proj = adapt_conv2d_in_channels(model.conv_proj, in_channels=1)
        in_features = model.heads.head.in_features
        head: nn.Module = nn.Linear(in_features, 4)
        if dropout > 0:
            head = nn.Sequential(nn.Dropout(p=dropout), head)
        model.heads.head = head
        return model

    raise ValueError(f"Unsupported backbone in checkpoint: {backbone}")


def export_onnx(checkpoint_path: Path, output_path: Path, input_size: int, opset: int) -> None:
    ckpt = torch.load(checkpoint_path, map_location="cpu")
    class_names = ckpt.get("class_names")
    if class_names and list(class_names) != class_names_expected:
        raise RuntimeError(f"Unexpected class order in checkpoint: {class_names}")

    backbone = ckpt.get("backbone", "densenet121")
    dropout = float(ckpt.get("dropout", 0.0))
    model = build_model(backbone, dropout=dropout)
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
    print(f"Backbone: {backbone}")
    print(f"Dropout: {dropout}")


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
