# Backend Observability Baseline

## What is implemented

- Structured JSON access logs (request id, route, status, latency, user id, client ip).
- Request correlation id middleware (`X-Request-Id`).
- OpenTelemetry tracing (OTLP HTTP exporter, configurable).
- gRPC tracing interceptors:
  - client-side injection in gateway -> microservice calls
  - server-side extraction in detection/report/governance services
- Metrics endpoint: `GET /metrics` (Prometheus text format).
- Inbound rate limiting and outbound circuit breaker/rate limiting for downstream protection.
- Built-in alert events in logs for:
  - HTTP `5xx`
  - slow requests (`ALERT_SLOW_REQUEST_MS`, default `2000`)
  - AI call failures (`AI_UNREACHABLE`, `AI_BAD_STATUS`, `AI_INVALID_PAYLOAD`)
  - readiness failure (`READINESS_FAILED`)

## Key environment variables

- `LOG_LEVEL` (`debug|info|warn|error`, default `info`)
- `ALERT_SLOW_REQUEST_MS` (default `2000`)
- `OTEL_ENABLED` (`true|false`)
- `OTEL_EXPORTER_OTLP_ENDPOINT` (e.g. `localhost:4318`)
- `OTEL_SERVICE_NAME` (default `cancer-detection-gateway`)
- `OTEL_TRACE_SAMPLE_RATIO` (default `1.0`)
- `INBOUND_RATE_LIMIT_RPS` / `INBOUND_RATE_LIMIT_BURST`
- `AI_OUTBOUND_RATE_LIMIT_RPS` / `AI_OUTBOUND_RATE_LIMIT_BURST`
- `UPSTREAM_BREAKER_FAIL_THRESHOLD`
- `UPSTREAM_BREAKER_OPEN_SECONDS`
- `UPSTREAM_BREAKER_HALF_OPEN_CALLS`

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

## Log Correlation Fields

- `request_id`: generated or forwarded request id.
- `trace_id`: OpenTelemetry trace id for distributed tracing.
- `span_id`: current request span id.
