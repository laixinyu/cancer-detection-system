# X-ray Lung Detection System

AI-assisted chest X-ray screening platform with role-based workflow for patient upload, doctor review, and report generation.

## Documentation Languages

- English: `README.md`
- 简体中文: `README.zh-CN.md`

## Current Status

- Frontend/Backend: Next.js + tRPC + Prisma + PostgreSQL
- AI Service: FastAPI + ONNX Runtime
- AI tasks:
  - A: Pneumonia risk
  - B: Nodule/Mass lesion risk
  - C: Infection coverage spectrum + white-lung quantification (screening-level)
- Core screening workflow:
  - Unified `screeningSummary` generated on upload (pneumonia + lesion + white-lung + triage priority)
  - Doctor queue prioritization uses screening triage when available
  - Review and report pages display structured screening metrics
- Compliance posture:
  - Heuristic pseudo bounding boxes are disabled by default
  - Output marked `RESEARCH_ONLY` (non-clinical use)
  - High-sensitivity and high-specificity decision thresholds supported
  - Clinical evidence ledger + governance gate + ops incident management added (admin)

## System Architecture

```mermaid
flowchart TB
  U1[Patient]
  U2[Doctor]
  U3[Admin]

  subgraph FE[Frontend Layer Next.js App Router]
    P1[Upload and Patient Pages]
    P2[Doctor Review Queue]
    P3[Reports Center]
    P4[Admin Dashboard]
    P5[Auth Pages]
  end

  subgraph BE[Application Layer Next.js API + tRPC]
    A1[NextAuth Authentication and Authorization]
    A2[Upload API /api/images/upload]
    A3[tRPC Routers user/image/detection/report/compliance/audit/analytics/ops]
    A4[Health Probes /api/health /api/ready]
  end

  subgraph DB[Data Layer]
    D1[(PostgreSQL + Prisma)]
    D2[(Redis)]
  end

  subgraph AI[AI Inference Layer FastAPI + ONNXRuntime]
    M1[Multitask Classifier Pneumonia Nodule Mass Opacity]
    M2[Detector Head ONNX Lesion Localization]
    M3[White-Lung and Infection Coverage Calculations]
    M4[/health and /predict]
  end

  subgraph FS[Storage Layer]
    F1[public/uploads image files]
    F2[ai-service/models ONNX models and configs]
  end

  U1 --> FE
  U2 --> FE
  U3 --> FE

  FE --> BE
  A2 --> F1
  A2 --> M4
  M4 --> M1
  M4 --> M2
  M4 --> M3
  M1 --> F2
  M2 --> F2

  A3 --> D1
  A3 --> D2
  A1 --> D1
  A4 --> D1
  A4 --> M4
```

### Architecture Summary

- Frontend: Next.js pages provide patient upload, doctor review, report viewing, and admin operations.
- Backend: Next.js API + tRPC handle business workflow; NextAuth enforces role-based access control.
- AI service: FastAPI provides `/predict` and `/health`, including screening scores, infection coverage, and white-lung assessment.
- Data: PostgreSQL stores core entities (users, images, detections, reports, audit, compliance, ops); Redis is available for cache/session scale.
- Storage: Uploaded images are saved under `public/uploads`, while ONNX models and AI configs are managed in `ai-service/models`.

## Quick Start

### 1. Install dependencies

```bash
npm install
```

### 2. Start infra and AI service

```bash
docker compose up -d
```

### 3. Setup database

```bash
npx prisma generate
npx prisma migrate dev --name init
```

### 4. Start app

```bash
npm run dev
```

App URL: `http://localhost:3000`

## AI Model Workflow

### Export multitask ONNX

```bash
cd ai-service
python -m venv .venv_export
.venv_export\Scripts\activate
pip install -r requirements-export.txt
python scripts/export_torchxrayvision_multitask_to_onnx.py --output ../models/cxr_multitask.onnx
```

Output channel order:
1. Pneumonia
2. Nodule
3. Mass
4. Lung Opacity

### Rebuild AI service

```bash
cd ..
docker compose up -d --build ai-service
```

### Health check

```bash
Invoke-RestMethod http://localhost:8000/health
```

Expected key fields:
- `modelLoaded: true`
- `modelPath: /app/models/cxr_multitask.onnx`
- `modelVersion: cxr-multitask-v1`
- `heuristicRegionsEnabled: false`

## Evaluation

Prepare CSV with columns:
- `y_true` (0/1)
- `y_score` (0~1)

Run:

```bash
python ai-service/scripts/evaluate_predictions.py --csv .\your_val.csv --thr-sens 0.30 --thr-spec 0.70 --out .\eval_report.json
```

Report includes:
- AUROC
- ECE (10 bins)
- High-sensitivity operating point metrics
- High-specificity operating point metrics

## Clinical Evidence And Ops Readiness

### Why this was added

- To reduce "clinical evidence不足": every validation run can be persisted with metrics and approvals.
- To reduce "production ops不足": readiness probe and incident tracking are available for admin operations.

### New backend capabilities

- Prisma model: `clinical_evidence_runs`
  - Tracks model version, dataset, sample/site size, AUROC/sensitivity/specificity, calibration/threshold metadata, regulatory status, stage recommendation.
- Prisma model: `ops_incidents`
  - Tracks incident severity (`P0~P3`), source, status (`OPEN/ACKNOWLEDGED/RESOLVED`), owner and timestamps.
- tRPC router: `ops`
  - `ops.readiness`: DB + AI liveness/readiness + latest evidence gate result.
  - `ops.dashboard`: latest evidence summary + open incidents + open P0/P1 incidents.
  - `ops.listEvidence` / `ops.createEvidence`
  - `ops.listIncidents` / `ops.createIncident` / `ops.transitionIncident`
- HTTP endpoints:
  - `GET /api/health`: app + DB health.
  - `GET /api/ready`: readiness for DB and AI service.

### Admin page updates

`/dashboard/admin` now shows:
- System readiness (DB/AI)
- Clinical evidence gate PASS/NOT PASS
- Open P0/P1 incidents
- Recent incident list

### Required DB migration

After pulling latest code, run:

```bash
npx prisma generate
npx prisma migrate dev --name add_clinical_evidence_and_ops
```

## Clinically-Oriented Parameter Tuning

Use validation data to derive calibration temperature and operating points instead of hardcoding.

Input CSV format:
- `task` in `pneumonia` or `lesion`
- `y_true` in `0/1`
- `y_score` in `0~1`
- optional `y_logit`

Run:

```bash
python ai-service/scripts/derive_clinical_config.py --csv .\clinical_val.csv --target-sens 0.95 --target-spec 0.90 --out .\ai-service\models\clinical_config.json
```

Then restart AI service:

```bash
docker compose up -d --build ai-service
```

Service will auto-load `AI_CLINICAL_CONFIG_PATH` (default `/app/models/clinical_config.json`).
Check `/health` for:
- `taskThresholds`
- `clinicalConfigSource`

### Clinical deployment gate

The service reads `AI_CLINICAL_GOVERNANCE_PATH` (default `/app/models/clinical_governance.json`).

- If governance file is missing/invalid, stage stays `RESEARCH_ONLY`.
- To move to decision-support stages, provide approved governance evidence.
- Stage is exposed via `/health` and `/predict` (`clinicalStage`).

Template:
- `ai-service/models/clinical_governance.example.json`

### Lesion localization upgrade path

- If `AI_DETECTOR_MODEL_PATH` exists (default `/app/models/cxr_detector.onnx`), service uses detector ONNX outputs for regions.
- Supported detector output styles:
  - `xyxy + score + class`
  - YOLO-like `cx,cy,w,h + class_probs`
- If detector model is absent, no regions are returned (or optional heuristic fallback if explicitly enabled).

### Detector output self-check (recommended before deployment)

```bash
python ai-service/scripts/check_detector_onnx.py --model ../models/cxr_detector.onnx --size 640
```

This script verifies:
- ONNX input/output tensor metadata
- Runtime output shape with a dummy tensor
- Whether the output layout is compatible with current decoder logic

### Detector manifest and hash pinning (recommended)

Generate profile + sha256:

```bash
python ai-service/scripts/generate_detector_manifest.py --model .\ai-service\models\cxr_detector.onnx --profile-out .\ai-service\models\detector_profile.json --sha-out .\ai-service\models\detector.sha256 --decoder-format auto --input-channels 1
```

Then set in `.env`:
- `AI_DETECTOR_PROFILE_PATH=/app/models/detector_profile.json`
- `AI_DETECTOR_SHA256=<content of detector.sha256>`

Startup will fail if model hash mismatches expected hash.

### Reproducibility check (same image, repeated inference)

```bash
python ai-service/scripts/check_detector_reproducibility.py --model .\ai-service\models\cxr_detector.onnx --image .\public\uploads\your_test_image.png --runs 20 --size 640 --out .\ai-service\models\repro_report.json
```

The script exits with non-zero code if max score/box drift exceeds configured eps.

### Train with NIH ChestXray14 (Google-hosted NIH dataset)

Dataset source you provided:
- `https://nihcc.app.box.com/v/ChestXray-NIHCC`

Expected local layout (after download/extract):
- `Data_Entry_2017.csv`
- image files under one or more subfolders (script will recursively scan)

Install training dependencies (outside Docker, local Python):

```bash
python -m pip install -r ai-service/requirements-train.txt
```

Optional: automated download + integrity check script

```bash
python ai-service/scripts/prepare_nih_chestxray14.py --dataset-root "E:\datasets\ChestXray-NIHCC" --download --extract --verify --report "E:\datasets\ChestXray-NIHCC\nih_integrity_report.json"
```

Notes:
- Download supports resume and stores archives in `dataset-root/downloads`.
- Extraction writes images into `dataset-root/images`.
- Verification checks archive readability and CSV/image coverage.

Train a multitask model aligned with current inference outputs:
- output order: `["Pneumonia", "Nodule", "Mass", "Lung Opacity"]`

```bash
python ai-service/scripts/train_nih_multitask.py --dataset-root "E:\datasets\ChestXray-NIHCC" --epochs 8 --batch-size 32 --output-dir "ai-service/models"
```

Export trained checkpoint to ONNX used by AI service:

```bash
python ai-service/scripts/export_trained_multitask_to_onnx.py --checkpoint "ai-service/models/nih_multitask_best.pt" --output "../models/cxr_multitask.onnx" --input-size 224
```

Restart AI service to load new model:

```bash
docker compose up -d --build ai-service
```

Verify:
- `GET http://localhost:8000/health` shows `modelVersion` and `modelLoaded=true`
- `POST /predict` returns `labelScores` with 4-task logits mapped by service.

### Startup gate (auto fail-fast)

The AI service now performs detector compatibility self-check at startup.

- `AI_ENFORCE_DETECTOR_STARTUP_CHECK=true` (default): if detector output is incompatible, service startup fails.
- `AI_ENFORCE_DETECTOR_STARTUP_CHECK=false`: service starts but detector is disabled.

Check gate status in `/health`:
- `detectorStartupCheckPassed`
- `detectorStartupCheckMessage`

### Governance gate (clinical stage guard)

The service enforces governance requirements before allowing decision-support stages.

Config:
- `AI_ENFORCE_GOVERNANCE_GATE=true`
- `AI_GOV_MIN_SITE_COUNT=2`
- `AI_GOV_MIN_AUROC=0.90`
- `AI_GOV_MIN_SENSITIVITY=0.90`
- `AI_GOV_MIN_SPECIFICITY=0.85`

Behavior:
- If `deploymentStage=RESEARCH_ONLY`, gate always passes.
- If `deploymentStage=PILOT_DECISION_SUPPORT` or `CLINICAL_DECISION_SUPPORT`, gate checks external validation metrics.
- For `CLINICAL_DECISION_SUPPORT`, regulatory status must be one of `APPROVED/CLEARED/CERTIFIED`.

Health endpoint fields:
- `governanceGatePassed`
- `governanceGateMessage`
- `governanceCriteria`

## Security and Access Rules

- Public registration allows `PATIENT` only.
- Patient can only access own images/detections/reports.
- Hard delete of images is disabled (audit integrity).
- Upload filename is server-generated (`timestamp + uuid + safe extension`).
- AI upload max size is enforced (default 10MB).
- Non-local production deployment requires non-default `NEXTAUTH_SECRET`.

## Main Commands

```bash
npm run dev
npm run build
npm run lint
npx prisma studio
docker compose up -d
docker compose logs -f ai-service
```

## Key Paths

- `app/` Next.js pages and API routes
- `server/` tRPC routers and compliance logic
- `prisma/schema.prisma` data models
- `ai-service/app/main.py` AI inference service
- `ai-service/scripts/` model export and evaluation scripts

## Notes

- This system is for research/decision support, not standalone clinical diagnosis.
- For full setup details and troubleshooting, see `SETUP.md`.
