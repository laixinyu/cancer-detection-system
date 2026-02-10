# X-ray Cancer Detection System

AI-assisted chest X-ray screening platform with role-based workflow for patient upload, doctor review, and report generation.

## Current Status

- Frontend/Backend: Next.js + tRPC + Prisma + PostgreSQL
- AI Service: FastAPI + ONNX Runtime
- AI tasks:
  - A: Pneumonia risk
  - B: Nodule/Mass lesion risk
- Compliance posture:
  - Heuristic pseudo bounding boxes are disabled by default
  - Output marked `RESEARCH_ONLY` (non-clinical use)
  - High-sensitivity and high-specificity decision thresholds supported

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
