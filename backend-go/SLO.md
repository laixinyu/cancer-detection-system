# 服务级 SLO

## 目标与范围

- 范围: `api-gateway`、`detection-service`、`report-service`、`governance-service`
- 周期: 30 天滚动窗口
- 模板: `backend-go/ops/templates/SERVICE_SLO_TEMPLATE.md`

## 服务级目标

1. `api-gateway`
- 可用性: >= 99.9%
- 平均延迟: < 400ms（不含上传与 AI 推理）
- 5xx 比例: < 0.5%

2. `detection-service`
- 可用性: >= 99.9%
- 平均延迟: < 500ms（查询接口）
- 错误率: < 1%

3. `report-service`
- 可用性: >= 99.9%
- 平均延迟: < 500ms
- 错误率: < 1%

4. `governance-service`
- 可用性: >= 99.9%
- 平均延迟: < 700ms
- 错误率: < 1%

## 指标来源与计算

每个服务独立 `/metrics`，关注：

1. `app_http_requests_total`
2. `app_http_request_duration_millis_sum`
3. `app_http_request_duration_millis_count`

计算建议：

1. 错误率：`sum(rate(5xx))/sum(rate(all))`
2. 平均延迟：`sum(rate(duration_sum))/sum(rate(duration_count))`

## 告警联动

- 告警规则文件：`backend-go/ops/alerts/prometheus-microservice-rules.yml`
- 故障演练模板：`backend-go/ops/templates/FAILOVER_DRILL_TEMPLATE.md`
