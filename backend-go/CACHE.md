# Backend Cache Design

## Current Implementation

- Cache abstraction: `internal/cache/cache.go`
- In-memory TTL cache: `internal/cache/memory.go`
- No-op cache: `internal/cache/noop.go`
- Runtime selection via env:
  - `CACHE_BACKEND=memory` (default)
  - `CACHE_BACKEND=noop`

## Cached Endpoints

- `GET /api/v1/images`
- `GET /api/v1/detections`
- `GET /api/v1/reports`
- `GET /api/v1/audits`
- `GET /api/v1/analytics/admin-overview`
- `GET /api/v1/ops/dashboard`
- `GET /api/v1/ops/evidence`
- `GET /api/v1/ops/incidents`

## Invalidation Strategy

Write operations proactively invalidate cache prefixes:

- Image upload / detection generation
- Detection review
- Report create / update
- Audit log write
- Ops evidence create
- Ops incident create / transition

## Redis Extension

The cache interface is ready for a Redis adapter.  
Add `internal/cache/redis.go` implementing `cache.Cache`, then wire it in `newApp` based on `CACHE_BACKEND=redis`.

