# 后端可观测性基线

## 已实现能力

- 结构化 JSON 访问日志（`request_id`、路由、状态码、耗时、用户 ID、客户端 IP）。
- 请求关联 ID 中间件（`X-Request-Id`）。
- OpenTelemetry 链路追踪（OTLP HTTP 导出，可配置）。
- gRPC 追踪拦截器：
  - 网关客户端侧注入 trace context
  - detection/report/governance 服务端提取 trace context
  - 通过 gRPC metadata 透传 `x-request-id` 以便日志关联
- 指标接口：`GET /metrics`（Prometheus 文本格式）。
- detection/report/governance 服务同样提供独立 `GET /metrics`。
- 入站限流与出站熔断/限流（保护下游依赖）。
- 内置告警日志事件：
  - HTTP `5xx`
  - 慢请求（`ALERT_SLOW_REQUEST_MS`，默认 `2000`）
  - AI 调用失败（`AI_UNREACHABLE`、`AI_BAD_STATUS`、`AI_INVALID_PAYLOAD`）
  - 就绪性失败（`READINESS_FAILED`）

## 关键环境变量

- `LOG_LEVEL`（`debug|info|warn|error`，默认 `info`）
- `ALERT_SLOW_REQUEST_MS`（默认 `2000`）
- `OTEL_ENABLED`（`true|false`）
- `OTEL_EXPORTER_OTLP_ENDPOINT`（例如 `localhost:4318`）
- `OTEL_SERVICE_NAME`（默认 `cancer-detection-gateway`）
- `OTEL_TRACE_SAMPLE_RATIO`（默认 `1.0`）
- `MICROSERVICE_MODE`（`strict|compat`）
- `INBOUND_RATE_LIMIT_RPS` / `INBOUND_RATE_LIMIT_BURST`
- `AI_OUTBOUND_RATE_LIMIT_RPS` / `AI_OUTBOUND_RATE_LIMIT_BURST`
- `UPSTREAM_BREAKER_FAIL_THRESHOLD`
- `UPSTREAM_BREAKER_OPEN_SECONDS`
- `UPSTREAM_BREAKER_HALF_OPEN_CALLS`

## 核心指标

- `app_http_requests_total{method,route,status}`
- `app_http_request_duration_millis_sum{method,route,status}`
- `app_http_request_duration_millis_count{method,route,status}`
- `app_ai_requests_total`
- `app_ai_failures_total`

建议按服务实例分别采集与聚合，不再仅看网关维度。

## 告警规则建议（Prometheus）

- 5 分钟窗口内 `5xx` 比例过高
- 10 分钟窗口内 P95 延迟高于 SLO
- 5 分钟窗口内 AI 失败率高于阈值
- `/ready` 连续 3 次检查不通过

## 日志关联字段

- `request_id`：系统生成或上游透传的请求 ID。
- `trace_id`：分布式链路追踪的 Trace ID。
- `span_id`：当前请求 Span ID。
- gRPC 服务端日志包含 `request_id + trace_id + span_id + method + latency_ms`。
