# Backend Architecture (Gin Microservices)

## Service Topology

- `api-gateway` (`cmd/server`): auth, image upload/file access, JWT middleware, proxy and compatibility routing.
- `detection-service` (`cmd/detection-service`): detection query/review domain.
- `report-service` (`cmd/report-service`): report query/create/update domain.
- `governance-service` (`cmd/governance-service`): audit, analytics, ops domain.

## Backend Architecture Diagram

```mermaid
flowchart LR
  FE[Next.js Frontend] --> GW[API Gateway cmd/server]

  subgraph Gateway
    GW --> MW[Cross-cutting middleware\nCORS JWT timeout logging]
    MW --> RT[Route dispatch /api/v1/*]
    RT --> AUTH[Auth + Upload + Image file]
    RT --> PX[Proxy to microservices]
  end

  PX --> DET[detection-service :8081]
  PX --> REP[report-service :8082]
  PX --> GOV[governance-service :8083]

  AUTH --> PG[(PostgreSQL)]
  AUTH --> CACHE[(Cache: memory/redis-ready)]
  DET --> PG
  REP --> PG
  GOV --> PG
  GOV --> CACHE
  AUTH --> AI[AI Service FastAPI /predict]
```

## AI Call Boundary

- Frontend must call backend APIs only.
- AI service is called by backend server-side handlers/services (for example image upload detection flow).
- Do not expose direct frontend -> ai-service integration in page code.
- Recommended deployment: keep `ai-service` on internal network; backend accesses it via `AI_SERVICE_URL`.

## Layered Design (Per Service)

```mermaid
flowchart TB
  H[handler\nGin route + DTO + auth check]
  S[service\nbusiness rules + transaction boundary]
  R[repository\nGORM data access]
  D[domain\nentity models in internal/domain]
  DB[(PostgreSQL)]
  C[(Cache)]

  H --> S --> R --> D --> DB
  S --> C
```

## Gateway Routing

Gateway keeps API paths unchanged (`/api/v1/*`) and proxies by env:

- `DETECTION_SERVICE_URL=http://localhost:8081`
- `REPORT_SERVICE_URL=http://localhost:8082`
- `GOVERNANCE_SERVICE_URL=http://localhost:8083`

If env var is empty, gateway falls back to local handler for compatibility.

## Current Refactor Status

- ORM models are centralized in `internal/domain`.
- `audit` has been split into `repository/service/handler` and wired into gateway app DI.
- Detection/report/governance standalone binaries are scaffolded and healthy.
- Remaining domains are being migrated incrementally to full 3-layer service modules.

## Observability

- Baseline observability design and metrics are documented in:
  - `backend-go/OBSERVABILITY.md`
