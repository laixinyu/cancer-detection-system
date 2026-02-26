param(
  [string]$Root = "."
)

$ErrorActionPreference = "Stop"
$repoPath = Join-Path $Root "internal/repository"
if (-not (Test-Path $repoPath)) {
  Write-Error "Repository path not found: $repoPath"
}

$violations = @()

function Check-ForbiddenPattern {
  param(
    [string]$File,
    [string[]]$Forbidden
  )
  foreach ($f in $Forbidden) {
    $matches = Select-String -Path $File -Pattern $f -SimpleMatch
    if ($matches) {
      $violations += "$File -> forbidden pattern: $f"
    }
  }
}

Check-ForbiddenPattern (Join-Path $repoPath "detection.go") @("ops_incidents", "clinical_evidence_runs")
Check-ForbiddenPattern (Join-Path $repoPath "report.go") @("ops_incidents", "clinical_evidence_runs")
Check-ForbiddenPattern (Join-Path $repoPath "ops.go") @("UPDATE reports", "INSERT INTO reports", "UPDATE detections", "INSERT INTO detections")

if ($violations.Count -gt 0) {
  Write-Host "Boundary violations found:" -ForegroundColor Red
  $violations | ForEach-Object { Write-Host " - $_" }
  exit 1
}

Write-Host "Repository boundary check passed."
