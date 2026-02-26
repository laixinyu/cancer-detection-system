# 真微服务演进路线（执行版）

## 目标

从“网关可回退本地业务 + 共享数据库”演进为“网关仅路由 + 每服务独立数据边界”。

## 已落地（本次）

1. 网关新增 `MICROSERVICE_MODE`：
   - `strict`：禁止 detection/report/governance 本地回退，未配置上游即 `503`。
   - `compat`：保留回退，便于过渡。
2. 严格模式配置校验：
   - 启动时要求每个域至少配置一个上游（gRPC 或 HTTP）。
3. 服务级独立 DSN 入口：
   - `GATEWAY_DATABASE_URL`
   - `DETECTION_DATABASE_URL`
   - `REPORT_DATABASE_URL`
   - `GOVERNANCE_DATABASE_URL`
   - 已移除网关对共享 `DATABASE_URL` 的依赖
4. 数据库初始化脚本：
   - `deploy/postgres/init/001_microservice_databases.sql`
   - 创建四个独立数据库与最小权限账号
5. 网关写链路幂等与事件化基础：
   - 幂等中间件（`Idempotency-Key`）
   - outbox 事件落库与异步 relay
   - 事件总线支持 `log|redis`，失败重试并标记死信状态
   - governance-service 可选 Redis Stream consumer（消费并记审计）
6. 三服务独立请求指标：
   - detection/report/governance 均提供 `/metrics`
7. 服务级治理文档模板：
   - `backend-go/SLO.md`
   - `backend-go/ops/alerts/prometheus-microservice-rules.yml`
   - `backend-go/ops/templates/SERVICE_SLO_TEMPLATE.md`
   - `backend-go/ops/templates/FAILOVER_DRILL_TEMPLATE.md`
8. 存储边界收敛：
   - 上传与读取改为 S3/MinIO 对象存储
   - 移除对本地 `public/uploads` 的运行时依赖

## 下一步（持续优化）

## 阶段 A：路由与发布边界（已基本完成，持续收敛）

1. 生产环境统一 `MICROSERVICE_MODE=strict`。
2. 将 detection/report/governance 三域 API 请求量、错误率按服务维度独立监控。
3. 网关仅保留认证、上传、文件访问与跨域编排。

## 阶段 B：数据边界（已完成主干，持续强化权限）

1. 为四服务配置独立数据库实例（或至少独立 schema + 最小权限账号）。
2. 审计每个服务的 repository，确保只访问本域表。
3. 维持“仅服务级 DSN”，禁止回退共享 `DATABASE_URL`。

## 阶段 C：一致性与协作（进行中）

1. 引入事件总线（outbox + consumer）替代跨域同步写。
2. 定义跨域事件契约（版本化，向后兼容）。
3. 对关键写链路补幂等键、重试与死信处理。

## 阶段 D：治理（进行中）

1. 每服务独立 SLO、容量、告警、发布窗口。
2. 按服务进行故障演练（降级、熔断、隔离）。
3. 建立“服务所有权”文档（Owner、依赖、变更影响面）。
