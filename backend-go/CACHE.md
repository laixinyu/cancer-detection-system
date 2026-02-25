# 后端缓存设计

## 当前实现

- 缓存抽象接口：`internal/cache/cache.go`
- 内存 TTL 缓存：`internal/cache/memory.go`
- Redis 缓存：`internal/cache/redis.go`
- 空实现缓存（No-op）：`internal/cache/noop.go`
- 通过环境变量选择运行时后端：
  - `CACHE_BACKEND=redis`
  - `CACHE_BACKEND=memory`
  - `CACHE_BACKEND=noop`
  - 默认策略：
    - `APP_ENV=production|prod` 且未显式设置 `CACHE_BACKEND` 时，默认 `redis`
    - 其他环境默认 `memory`
  - Redis 参数：
    - `REDIS_ADDR`（默认 `localhost:6379`）
    - `REDIS_PASSWORD`（可空）
    - `REDIS_DB`（默认 `0`）

## 已缓存接口

- `GET /api/v1/detections`
- `GET /api/v1/reports`
- `GET /api/v1/audits`
- `GET /api/v1/analytics/admin-overview`
- `GET /api/v1/ops/dashboard`
- `GET /api/v1/ops/evidence`
- `GET /api/v1/ops/incidents`

## 失效策略

写操作会主动按前缀清理缓存，包含：

- 检测复核
- 报告创建 / 更新
- 审计日志写入
- 运维证据创建
- 运维事件创建 / 状态流转

## 缓存边界

- 仅对读多写少、允许短 TTL 最终一致的查询接口启用缓存。
- 以下强一致或安全敏感路径不启用缓存：
  - 认证（登录、鉴权、权限校验）
  - 上传与文件入库流程
  - AI 推理调用链路（`/predict`）
