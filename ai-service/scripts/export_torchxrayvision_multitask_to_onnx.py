import argparse
import inspect
from pathlib import Path
from typing import List

import torch
import torchxrayvision as xrv


TARGET_PATHOLOGIES = ["Pneumonia", "Nodule", "Mass", "Lung Opacity"]


class MultiTaskWrapper(torch.nn.Module):
    def __init__(self, base_model: torch.nn.Module) -> None:
        super().__init__()
        index_map = {name: idx for idx, name in enumerate(base_model.pathologies)}
        selected: List[int] = []
        missing: List[str] = []

        for pathology in TARGET_PATHOLOGIES:
            idx = index_map.get(pathology)
            if idx is None:
                missing.append(pathology)
            else:
                selected.append(idx)

        if missing:
            raise RuntimeError(f"Missing pathology heads in source model: {missing}")

        self.base_model = base_model
        self.register_buffer("selected_indices", torch.tensor(selected, dtype=torch.long))

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        logits = self.base_model(x)
        selected_logits = torch.index_select(logits, dim=1, index=self.selected_indices)
        return selected_logits


def export_onnx(weights: str, output_path: Path, input_size: int, opset: int) -> None:
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

    wrapped = MultiTaskWrapper(base_model)
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
        output_names=["task_logits"],
        dynamic_axes={"input": {0: "batch"}, "task_logits": {0: "batch"}},
    )

    print(f"Exported ONNX model to: {output_path}")
    print(f"Weights source: {weights}")
    print(f"Input size: {input_size}x{input_size}")
    print(f"Output channels order: {TARGET_PATHOLOGIES}")


def main() -> None:
    parser = argparse.ArgumentParser(description="Export TorchXRayVision multi-task model to ONNX.")
    parser.add_argument("--weights", default="densenet121-res224-all")
    parser.add_argument("--output", default="../models/cxr_multitask.onnx")
    parser.add_argument("--input-size", type=int, default=224)
    parser.add_argument("--opset", type=int, default=17)
    args = parser.parse_args()

    script_dir = Path(__file__).resolve().parent
    output_path = (script_dir / args.output).resolve()
    export_onnx(args.weights, output_path, args.input_size, args.opset)


if __name__ == "__main__":
    main()
