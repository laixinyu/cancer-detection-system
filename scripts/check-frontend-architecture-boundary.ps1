$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
$routerPath = Join-Path $root 'server\routers'

if (-not (Test-Path $routerPath)) {
  Write-Host "server/routers not found, skip."
  exit 0
}

$patterns = @(
  'ctx\.prisma',
  "from '@prisma/client'",
  "from '@/lib/prisma'"
)

$violations = @()
foreach ($pattern in $patterns) {
  $matches = rg -n $pattern $routerPath 2>$null
  if ($LASTEXITCODE -eq 0 -and $matches) {
    $violations += $matches
  }
}

if ($violations.Count -gt 0) {
  $details = $violations -join "`n"
  Write-Error "Architecture boundary violation: frontend routers must not access Prisma directly.`n$details"
  exit 1
}

Write-Host "Architecture boundary check passed."
