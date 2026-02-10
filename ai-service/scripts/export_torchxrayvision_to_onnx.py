import argparse
import inspect
from pathlib import Path

import torch
import torchxrayvision as xrv


class CancerDetectionWrapper(torch.nn.Module):
    """Wrap TorchXRayVision output to a single cancer probability logit."""

    def __init__(self, base_model: torch.nn.Module) -> None:
        super().__init__()
        self.base_model = base_model
        pathology_to_idx = {name: idx for idx, name in enumerate(base_model.pathologies)}

        mass_idx = pathology_to_idx.get("Mass")
        nodule_idx = pathology_to_idx.get("Nodule")
        lesion_idx = pathology_to_idx.get("Lung Lesion")
        opacity_idx = pathology_to_idx.get("Lung Opacity")

        selected = [idx for idx in [mass_idx, nodule_idx, lesion_idx, opacity_idx] if idx is not None]
        if not selected:
            raise RuntimeError("Expected pathology heads (Mass/Nodule/Lung Lesion/Lung Opacity) not found.")
        self.register_buffer("selected_indices", torch.tensor(selected, dtype=torch.long))

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        logits = self.base_model(x)
        selected_logits = torch.index_select(logits, dim=1, index=self.selected_indices)
        # Probability of cancer-like finding = max over selected pathology probabilities.
        cancer_prob, _ = torch.max(torch.sigmoid(selected_logits), dim=1, keepdim=True)
        # Convert back to logit-like space so caller can apply sigmoid if needed.
        eps = 1e-6
        cancer_prob = torch.clamp(cancer_prob, eps, 1 - eps)
        cancer_logit = torch.log(cancer_prob / (1 - cancer_prob))
        return cancer_logit


def export_onnx(weights: str, output_path: Path, input_size: int, opset: int) -> None:
    # PyTorch 2.6 changed torch.load default(weights_only=True), while some
    # torchxrayvision weights rely on full checkpoint deserialization.
    original_torch_load = torch.load
    signature = inspect.signature(original_torch_load)
    has_weights_only = "weights_only" in signature.parameters

    if has_weights_only:
        def patched_torch_load(*args, **kwargs):
            kwargs.setdefault("weights_only", False)
            return original_torch_load(*args, **kwargs)

        torch.load = patched_torch_load

    base_model = xrv.models.DenseNet(weights=weights)
    torch.load = original_torch_load
    wrapped = CancerDetectionWrapper(base_model)
    wrapped.eval()

    dummy = torch.randn(1, 1, input_size, input_size, dtype=torch.float32)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    torch.onnx.export(
        wrapped,
        dummy,
        str(output_path),
        export_params=True,
        opset_version=opset,
        do_constant_folding=True,
        input_names=["input"],
        output_names=["cancer_logit"],
        dynamic_axes={"input": {0: "batch"}, "cancer_logit": {0: "batch"}},
    )

    print(f"Exported ONNX model to: {output_path}")
    print(f"Weights source: {weights}")
    print(f"Input size: {input_size}x{input_size}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Export TorchXRayVision model to ONNX for cancer probability.")
    parser.add_argument("--weights", default="densenet121-res224-all")
    parser.add_argument("--output", default="../models/cancer_detector.onnx")
    parser.add_argument("--input-size", type=int, default=224)
    parser.add_argument("--opset", type=int, default=17)
    args = parser.parse_args()

    script_dir = Path(__file__).resolve().parent
    output_path = (script_dir / args.output).resolve()
    export_onnx(args.weights, output_path, args.input_size, args.opset)


if __name__ == "__main__":
    main()
