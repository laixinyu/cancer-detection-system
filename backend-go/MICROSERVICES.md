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
    GW --> MW[Cross-cutting middleware\nCORS JWT timeout rate-limit trace logging]
    MW --> RT[Route dispatch /api/v1/*]
    RT --> AUTH[Auth + Upload + Image file]
    RT --> PX[Proxy to microservices]
    AUTH --> RS[Resilience\nCircuit Breaker + Outbound Rate Limit]
    AUTH --> OBS[Observability\nMetrics + Trace + Request ID]
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

## Composition Root and Dependency Injection

- Gateway DI composition root: `internal/platform/bootstrap/gateway.go`
- Services and repositories are constructed once in composition root, then injected into handlers.
- Handlers do not create DB connections, cache clients, or repositories inside request logic.
- This keeps test seams clear and prevents hidden runtime coupling.

## Resilience

- Inbound rate limiter at gateway middleware level (`429` protection).
- Outbound protection to AI and HTTP upstream:
  - keyed token-bucket rate limiting
  - circuit breaker (closed/open/half-open)
- Configurable by env:
  - `INBOUND_RATE_LIMIT_RPS`, `INBOUND_RATE_LIMIT_BURST`
  - `AI_OUTBOUND_RATE_LIMIT_RPS`, `AI_OUTBOUND_RATE_LIMIT_BURST`
  - `UPSTREAM_BREAKER_FAIL_THRESHOLD`, `UPSTREAM_BREAKER_OPEN_SECONDS`, `UPSTREAM_BREAKER_HALF_OPEN_CALLS`

## Tracing and Logging Correlation

- OpenTelemetry tracing bootstrap: `internal/observability/tracing.go`
- Gin trace middleware opens spans per request and propagates upstream trace context.
- Structured logs include:
  - `request_id`
  - `trace_id`
  - `span_id`

## Gateway Routing

Gateway keeps API paths unchanged (`/api/v1/*`) and proxies by env:

- gRPC (preferred):
  - `DETECTION_SERVICE_GRPC_ADDR=detection-service:9081`
  - `REPORT_SERVICE_GRPC_ADDR=report-service:9082`
  - `GOVERNANCE_SERVICE_GRPC_ADDR=governance-service:9083`
- `DETECTION_SERVICE_URL=http://localhost:8081`
- `REPORT_SERVICE_URL=http://localhost:8082`
- `GOVERNANCE_SERVICE_URL=http://localhost:8083`

If gRPC addr is configured, gateway uses gRPC for service-to-service calls.
If both gRPC and HTTP are empty, gateway falls back to local handler for compatibility.

## Current Refactor Status

- ORM models are centralized in `internal/domain`.
- Domain handlers are split as `repository/service/handler` and wired through DI.
- Detection/report/governance binaries run with unified HTTP+gRPC lifecycle and graceful shutdown.
- Gateway retains local fallback handlers for compatibility when upstream URLs are not configured.

## Observability

- Baseline observability design and metrics are documented in:
  - `backend-go/OBSERVABILITY.md`

## API Documentation

- Backend API reference:
  - `backend-go/API.md`
