# AGENTS.md

## Cursor Cloud specific instructions

### Overview

This is an AI-assisted Chest X-ray Lung Detection System with two main runtime services:
- **Next.js web app** (TypeScript, React 19, tRPC, Prisma, NextAuth) on port 3000
- **Python FastAPI AI inference service** (ONNX Runtime) on port 8000

Backed by **PostgreSQL 16** (via Docker).

### Services and how to run them

See `README.md` "Quick Start" for the standard commands. Key points:

| Service | Start Command | Port |
|---|---|---|
| PostgreSQL | `docker compose up -d postgres` | 5432 |
| AI service (native) | `cd ai-service && .venv/bin/uvicorn app.main:app --host 0.0.0.0 --port 8000` | 8000 |
| Next.js dev | `npm run dev` | 3000 |

### Non-obvious caveats

- **Docker daemon**: The sandbox runs inside a Firecracker VM container; Docker must be started manually with `sudo dockerd &>/tmp/dockerd.log &` before using `docker compose`. The daemon config at `/etc/docker/daemon.json` uses `fuse-overlayfs` storage driver and iptables-legacy is required.
- **AI service fallback**: If the ONNX model file is missing, the AI service uses a deterministic texture-based fallback (not random). The main classifier model (`ai-service/models/cxr_multitask.onnx`) ships in the repo. The detector model may show a startup check warning — this is expected in dev and doesn't block the service.
- **Environment variables**: A `.env` file is needed at the workspace root with `DATABASE_URL`, `NEXTAUTH_SECRET`, `NEXTAUTH_URL`, `AI_SERVICE_URL`. See the docker-compose.yml for PostgreSQL credentials (`cancer_user`/`cancer_password`/`cancer_detection`).
- **Prisma migrations**: After pulling changes with new schema updates, run `npx prisma generate && npx prisma migrate dev` before starting the Next.js app. The `migrate dev` command is interactive; use `--name <name>` to skip the prompt.
- **Python venv**: The AI service Python dependencies are in `ai-service/.venv`. The Dockerfile uses Python 3.11-slim but the system Python 3.12 works fine natively.
- **Lint**: `npm run lint` — 0 errors expected, some warnings about `<img>` and React Hook deps are known.
- **Build**: `npm run build` — should succeed cleanly.
- **Package manager**: npm (lockfile is `package-lock.json`).
