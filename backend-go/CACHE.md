# 后端缓存设计

## 当前实现

- 缓存抽象接口：`internal/cache/cache.go`
- 内存 TTL 缓存：`internal/cache/memory.go`
- 空实现缓存（No-op）：`internal/cache/noop.go`
- 通过环境变量选择运行时后端：
  - `CACHE_BACKEND=memory`（默认）
  - `CACHE_BACKEND=noop`

## 已缓存接口

- `GET /api/v1/images`
- `GET /api/v1/detections`
- `GET /api/v1/reports`
- `GET /api/v1/audits`
- `GET /api/v1/analytics/admin-overview`
- `GET /api/v1/ops/dashboard`
- `GET /api/v1/ops/evidence`
- `GET /api/v1/ops/incidents`

## 失效策略

写操作会主动按前缀清理缓存，包含：

- 影像上传 / 检测生成
- 检测复核
- 报告创建 / 更新
- 审计日志写入
- 运维证据创建
- 运维事件创建 / 状态流转

## Redis 扩展

当前缓存接口已支持扩展 Redis 适配器。  
新增 `internal/cache/redis.go` 并实现 `cache.Cache` 后，可在 `newApp` 中根据 `CACHE_BACKEND=redis` 挂载。
