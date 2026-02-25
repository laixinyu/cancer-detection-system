# Backend Observability Baseline

## What is implemented

- Structured JSON access logs (request id, route, status, latency, user id, client ip).
- Request correlation id middleware (`X-Request-Id`).
- Metrics endpoint: `GET /metrics` (Prometheus text format).
- Built-in alert events in logs for:
  - HTTP `5xx`
  - slow requests (`ALERT_SLOW_REQUEST_MS`, default `2000`)
  - AI call failures (`AI_UNREACHABLE`, `AI_BAD_STATUS`, `AI_INVALID_PAYLOAD`)
  - readiness failure (`READINESS_FAILED`)

## Key environment variables

- `LOG_LEVEL` (`debug|info|warn|error`, default `info`)
- `ALERT_SLOW_REQUEST_MS` (default `2000`)

## Core metrics

- `app_http_requests_total{method,route,status}`
- `app_http_request_duration_millis_sum{method,route,status}`
- `app_http_request_duration_millis_count{method,route,status}`
- `app_ai_requests_total`
- `app_ai_failures_total`

## Suggested alert rules (Prometheus)

- High 5xx ratio for 5m
- P95 latency above SLO for 10m
- AI failure rate above threshold for 5m
- `/ready` not ready for continuous 3 checks

