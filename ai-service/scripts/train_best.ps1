param(
  [string]$DatasetRoot = "E:\datasets\ChestXray-NIHCC",
  [string]$ResizedRoot = "E:\datasets\ChestXray-NIHCC-512",
  [int]$Resize = 512,
  [switch]$SkipResize,
  [string]$Backbone = "efficientnet_v2_s",
  [int]$ImageSize = 320,
  [int]$Epochs = 8,
  [int]$BatchSize = 64,
  [int]$NumWorkers = 16,
  [int]$PrefetchFactor = 4,
  [string]$OutputDir = "ai-service/models",
  [switch]$ExportOnnx,
  [switch]$RebuildAi
)

$ErrorActionPreference = "Stop"

function Assert-LastExitCode([string]$StepName) {
  if ($LASTEXITCODE -ne 0) {
    throw "[train:best] $StepName failed with exit code $LASTEXITCODE"
  }
}

Write-Host "[train:best] dataset root: $DatasetRoot"
Write-Host "[train:best] resized root: $ResizedRoot"
Write-Host "[train:best] Step 0/3: verify CUDA runtime..."
python -c "import torch, sys; print('[train:best] torch=' + torch.__version__); print('[train:best] cuda=' + str(torch.version.cuda)); print('[train:best] cuda_available=' + str(torch.cuda.is_available())); print('[train:best] device=' + (torch.cuda.get_device_name(0) if torch.cuda.is_available() else 'CPU')); sys.exit(0 if torch.cuda.is_available() else 2)"
Assert-LastExitCode "CUDA preflight"

if (-not $SkipResize) {
  Write-Host "[train:best] Step 1/3: prepare resized dataset..."
  python ai-service/scripts/prepare_nih_resized_dataset.py `
    --dataset-root "$DatasetRoot" `
    --output-root "$ResizedRoot" `
    --size $Resize `
    --quality 90 `
    --workers $NumWorkers `
    --skip-existing
  Assert-LastExitCode "dataset resize"
} else {
  Write-Host "[train:best] Step 1/3: skip resize"
}

Write-Host "[train:best] Step 2/3: train model with best-practice defaults..."
python ai-service/scripts/train_nih_multitask.py `
  --dataset-root "$ResizedRoot" `
  --split-mode nih_official `
  --backbone $Backbone `
  --image-size $ImageSize `
  --epochs $Epochs `
  --batch-size $BatchSize `
  --loss-type focal `
  --focal-gamma 2.0 `
  --focal-alpha 0.25 `
  --class-loss-weights 1.6,1,1,1 `
  --augment `
  --augment-strength strong `
  --dropout 0.3 `
  --use-weighted-sampler `
  --sampler-power 0.5 `
  --sampler-max-weight 5.0 `
  --amp `
  --grad-clip 1.0 `
  --early-stop-patience 2 `
  --early-stop-min-delta 0.001 `
  --save-epoch-checkpoints `
  --num-workers $NumWorkers `
  --prefetch-factor $PrefetchFactor `
  --output-dir "$OutputDir"
Assert-LastExitCode "model training"

$checkpoint = if ($Backbone -eq "densenet121") {
  Join-Path $OutputDir "nih_multitask_best.pt"
} else {
  Join-Path $OutputDir ("nih_multitask_" + $Backbone + "_best.pt")
}

if ($ExportOnnx) {
  Write-Host "[train:best] Step 3/3: export ONNX..."
  python ai-service/scripts/export_trained_multitask_to_onnx.py `
    --checkpoint "$checkpoint" `
    --output "../models/cxr_multitask.onnx" `
    --input-size $ImageSize
  Assert-LastExitCode "onnx export"
} else {
  Write-Host "[train:best] Step 3/3: skip ONNX export (use -ExportOnnx to enable)"
}

if ($RebuildAi) {
  Write-Host "[train:best] Rebuild AI service..."
  docker compose up -d --build ai-service
}

Write-Host "[train:best] Done."
