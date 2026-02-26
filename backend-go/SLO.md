# 服务级 SLO（初版）

## API Gateway

1. 可用性: 99.9%
2. P95 延迟: < 400ms（不含上传/AI 推理）
3. 5xx 比例: < 0.5%

## Detection Service

1. 可用性: 99.9%
2. P95 延迟: < 500ms（查询接口）
3. 错误率: < 1%

## Report Service

1. 可用性: 99.9%
2. P95 延迟: < 500ms
3. 错误率: < 1%

## Governance Service

1. 可用性: 99.9%
2. P95 延迟: < 700ms
3. 错误率: < 1%

## 指标来源

每个服务独立 `/metrics`，关注：

1. `app_http_requests_total`
2. `app_http_request_duration_millis_sum`
3. `app_http_request_duration_millis_count`
