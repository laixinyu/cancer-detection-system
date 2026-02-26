# 跨域事件契约（v1）

## 通用 Envelope

所有域事件使用统一结构：

- `id`：事件唯一 ID（UUID）
- `version`：契约版本，当前 `v1`
- `eventType`：事件类型（如 `post.report.v1`）
- `aggregateType`：聚合类型（`report`/`detection`/`ops_incident` 等）
- `aggregateId`：聚合 ID
- `payload`：事件负载（JSON）
- `idempotencyKey`：可选，来源请求幂等键
- `occurredAt`：事件发生时间（UTC）

## 发布通道

- `EVENT_BUS_BACKEND=log|redis`
- Redis Stream 模式：
  - `EVENT_BUS_REDIS_ADDR`
  - `EVENT_BUS_REDIS_PASSWORD`
  - `EVENT_BUS_REDIS_DB`
  - `EVENT_BUS_REDIS_STREAM`（默认 `domain_events`）

## 消费端（governance-service）

- `EVENT_CONSUMER_ENABLED=true|false`
- `EVENT_CONSUMER_GROUP`（默认 `governance`）
- `EVENT_CONSUMER_NAME`（默认 `governance-1`）

## 版本化策略

1. 新增字段只能追加，禁止破坏性重命名。
2. 变更语义时提升 `version`（如 `v2`）。
3. 消费方必须容忍未知字段。
