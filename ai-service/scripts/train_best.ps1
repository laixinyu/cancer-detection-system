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
  [switch]$RunValidationPipeline,
  [string]$ValidationOutputDir = "ai-service/models/eval",
  [string]$ValidationSplit = "val",
  [double]$ThresholdHighSens = 0.30,
  [double]$ThresholdHighSpec = 0.70,
  [double]$TargetSens = 0.95,
  [double]$TargetSpec = 0.90,
  [string]$ClinicalConfigOut = "ai-service/models/clinical_config.json",
  [switch]$AllowCpu,
  [string]$PythonExe = "",
  [switch]$ExportOnnx,
  [switch]$RebuildAi
)

$ErrorActionPreference = "Stop"
$PythonCmd = if ([string]::IsNullOrWhiteSpace($PythonExe)) { "python" } else { $PythonExe }

function Assert-LastExitCode([string]$StepName) {
  if ($LASTEXITCODE -ne 0) {
    throw "[train:best] $StepName failed with exit code $LASTEXITCODE"
  }
}

Write-Host "[train:best] dataset root: $DatasetRoot"
Write-Host "[train:best] resized root: $ResizedRoot"
Write-Host "[train:best] Step 0/3: verify CUDA runtime..."
& $PythonCmd -c "import torch, sys; print('[train:best] torch=' + torch.__version__); print('[train:best] cuda=' + str(torch.version.cuda)); print('[train:best] cuda_available=' + str(torch.cuda.is_available())); print('[train:best] device=' + (torch.cuda.get_device_name(0) if torch.cuda.is_available() else 'CPU')); sys.exit(0 if torch.cuda.is_available() else 2)"
if ($LASTEXITCODE -ne 0) {
  if ($AllowCpu) {
    Write-Warning "[train:best] CUDA unavailable, continue on CPU because -AllowCpu is set. Training will be very slow."
  } else {
    throw "[train:best] CUDA preflight failed. Current Python is CPU-only. If you intentionally run on CPU, add -AllowCpu. Otherwise activate your GPU training env and rerun."
  }
}

if (-not $SkipResize) {
  Write-Host "[train:best] Step 1/3: prepare resized dataset..."
  & $PythonCmd ai-service/scripts/prepare_nih_resized_dataset.py `
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
if ($AllowCpu) {
  & $PythonCmd ai-service/scripts/train_nih_multitask.py `
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
    --grad-clip 1.0 `
    --early-stop-patience 2 `
    --early-stop-min-delta 0.001 `
    --save-epoch-checkpoints `
    --num-workers $NumWorkers `
    --prefetch-factor $PrefetchFactor `
    --output-dir "$OutputDir" `
    --cpu
} else {
  & $PythonCmd ai-service/scripts/train_nih_multitask.py `
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
}
Assert-LastExitCode "model training"

$checkpoint = if ($Backbone -eq "densenet121") {
  Join-Path $OutputDir "nih_multitask_best.pt"
} else {
  Join-Path $OutputDir ("nih_multitask_" + $Backbone + "_best.pt")
}

if ($ExportOnnx) {
  Write-Host "[train:best] Step 3/3: export ONNX..."
  & $PythonCmd ai-service/scripts/export_trained_multitask_to_onnx.py `
    --checkpoint "$checkpoint" `
    --output "../models/cxr_multitask.onnx" `
    --input-size $ImageSize
  Assert-LastExitCode "onnx export"
} else {
  Write-Host "[train:best] Step 3/3: skip ONNX export (use -ExportOnnx to enable)"
}

if ($RunValidationPipeline) {
  Write-Host "[train:best] Running validation pipeline (export -> evaluate -> calibration)..."
  & $PythonCmd ai-service/scripts/run_validation_pipeline.py `
    --dataset-root "$ResizedRoot" `
    --checkpoint "$checkpoint" `
    --split-mode nih_official `
    --target-split "$ValidationSplit" `
    --output-dir "$ValidationOutputDir" `
    --batch-size $BatchSize `
    --num-workers $NumWorkers `
    --thr-sens $ThresholdHighSens `
    --thr-spec $ThresholdHighSpec `
    --target-sens $TargetSens `
    --target-spec $TargetSpec `
    --clinical-config-out "$ClinicalConfigOut"
  Assert-LastExitCode "validation pipeline"
}

if ($RebuildAi) {
  Write-Host "[train:best] Rebuild AI service..."
  docker compose up -d --build ai-service
}

Write-Host "[train:best] Done."
