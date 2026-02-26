# 服务级 SLO 模板

## 基本信息

- 服务名:
- Owner:
- 生效日期:
- 发布窗口:

## SLI 与目标

1. 可用性（Availability）
- SLI: `1 - (5xx / all_requests)`
- 目标: `>= 99.9%`（30d）

2. 延迟（Latency）
- SLI: `avg(request_duration_millis)`（10m rolling）
- 目标: `< xxx ms`

3. 正确性（Correctness）
- SLI: 业务错误率（按服务自定义，如状态迁移失败率）
- 目标: `< x%`

## 错误预算

- 周期: 30 天
- 预算: `100% - SLO`
- 预算耗尽策略:
  - 冻结非紧急发布
  - 优先处理可靠性缺陷

## 观测与告警绑定

- 指标源:
  - `app_http_requests_total`
  - `app_http_request_duration_millis_sum`
  - `app_http_request_duration_millis_count`
- 告警规则文件:
  - `backend-go/ops/alerts/prometheus-microservice-rules.yml`

## 例外与降级策略

- 可接受降级:
- 不可接受降级:
- 熔断/限流策略:

## 审核记录

- 审核人:
- 最近复审时间:
- 复审结论:
