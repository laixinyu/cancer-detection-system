# 后端 API 文档（Gin）

## OpenAPI 文件

- YAML: `backend-go/openapi.yaml`
- JSON: `backend-go/openapi.json`
- 可直接导入 Swagger UI / Postman

## 1. 基本信息

- 网关基址：`http://localhost:8080`
- 统一前缀：`/api/v1`
- 鉴权方式：`Authorization: Bearer <JWT>`
- 响应格式：`application/json`
- 追踪头：`X-Request-Id`（服务端会回传）
- 链路追踪：支持 W3C Trace Context（`traceparent`）
- 内部通信：网关到 `detection/report/governance` 微服务优先使用 gRPC

## 2. 公共系统接口（无需登录）

### `GET /health`

- 说明：存活检查
- 返回示例：

```json
{
  "status": "ok",
  "service": "cancer-detection-backend-go",
  "timestamp": "2026-02-25T10:00:00Z"
}
```

### `GET /ready`

- 说明：就绪检查（DB + AI）
- 返回示例：

```json
{
  "status": "ready",
  "timestamp": "2026-02-25T10:00:00Z",
  "database": { "ready": true, "error": null },
  "ai": { "reachable": true, "status": "ok" }
}
```

### `GET /metrics`

- 说明：Prometheus 指标（`text/plain`）

## 3. 认证接口

### `POST /api/v1/auth/register`

- 说明：患者自注册（仅允许 `PATIENT`）
- 请求体：

```json
{
  "name": "patient name",
  "email": "patient@example.com",
  "password": "Patient#2026",
  "role": "PATIENT",
  "phone": "13800000000"
}
```

- 返回：`201`

```json
{
  "user": {
    "id": "uuid",
    "email": "patient@example.com",
    "name": "patient name",
    "role": "PATIENT"
  }
}
```

### `POST /api/v1/auth/login`

- 请求体：

```json
{
  "email": "patient@example.com",
  "password": "Patient#2026"
}
```

- 返回：`200`

```json
{
  "token": "jwt-token",
  "user": {
    "id": "uuid",
    "email": "patient@example.com",
    "name": "patient name",
    "role": "PATIENT"
  },
  "expiresAt": "2026-02-26T10:00:00Z"
}
```

## 4. 影像与检测

### `GET /api/v1/images`（需登录）

- 查询参数：`limit`（默认20）
- 返回：

```json
{
  "images": [
    {
      "id": "uuid",
      "patientId": "uuid",
      "filePath": "/api/images/{id}/file",
      "status": "COMPLETED",
      "detections": [
        {
          "id": "uuid",
          "cancerProbability": 0.42,
          "status": "PENDING",
          "modelVersion": "cxr-multitask-v1"
        }
      ]
    }
  ],
  "nextCursor": null
}
```

### `POST /api/v1/images/upload`（需登录）

- `multipart/form-data`
- 字段：
  - `file`: 文件（PNG/JPEG/TIFF/DCM，<=10MB）
  - `consentAccepted`: `true|false`
  - `consentVersion`: 如 `v1.0`

- 返回：

```json
{
  "success": true,
  "image": {
    "id": "uuid",
    "originalName": "xray.png",
    "filePath": "/api/images/{id}/file",
    "status": "COMPLETED"
  }
}
```

### `GET /api/v1/images/:id/file`（需登录）

- 说明：读取影像二进制内容
- 返回：文件流（按原始扩展名设置 `Content-Type`）

### `GET /api/v1/images/:id`（需登录）

- 说明：读取单张影像详情（含检测列表、患者概要）
- 返回：单条影像对象（字段与 `/images` 列表兼容）

### `GET /api/v1/detections`（需登录）

- 查询参数：
  - `status`: `PENDING|REVIEWED|CONFIRMED`
  - `priority`: `HIGH|MEDIUM|LOW`
  - `orderByPriority`: `true|false`（默认 `true`）
  - `limit`: 默认 `20`

- 返回：

```json
{
  "detections": [{ "id": "uuid", "status": "PENDING", "cancerProbability": 0.62 }],
  "nextCursor": null
}
```

### `GET /api/v1/detections/:id`（需登录）

- 返回：单条检测详情（含影像与患者概要）

### `POST /api/v1/detections/:id/review`（医生/管理员）

- 请求体：

```json
{
  "status": "REVIEWED",
  "reviewNotes": "looks good",
  "findings": {}
}
```

- 说明：允许状态流转
  - `PENDING -> REVIEWED|CONFIRMED`
  - `REVIEWED -> CONFIRMED`

## 5. 用户与合规

### `GET /api/v1/users/me`（需登录）

- 返回：当前登录用户资料

### `PATCH /api/v1/users/me`（需登录）

- 请求体（至少一项）：

```json
{
  "name": "new name",
  "phone": "13800000000"
}
```

### `GET /api/v1/compliance/clinical-scope`（需登录）

- 返回：产品临床适用边界说明

### `POST /api/v1/compliance/consents`（需登录）

- 请求体：

```json
{
  "consentType": "AI_ANALYSIS",
  "consentVersion": "v1.0"
}
```

### `GET /api/v1/compliance/consents/latest`（需登录）

- 查询参数：`consentType`（默认 `AI_ANALYSIS`）
- 返回：最新已签署同意记录（可能为 `null`）

## 6. 报告接口

### `GET /api/v1/reports`（需登录）

- 查询参数：`status`、`patientId`、`limit`
- 返回：

```json
{
  "reports": [{ "id": "uuid", "status": "DRAFT" }],
  "nextCursor": null
}
```

### `GET /api/v1/reports/:id`（需登录）

- 返回：报告详情（含 patient/doctor/detection）

### `POST /api/v1/reports`（医生/管理员）

- 请求体：

```json
{
  "detectionId": "uuid",
  "patientId": "uuid",
  "content": {},
  "status": "DRAFT"
}
```

- 说明：按 `detectionId` 幂等 upsert

### `PATCH /api/v1/reports/:id`（医生/管理员）

- 请求体（至少一项）：

```json
{
  "content": {},
  "status": "FINALIZED",
  "pdfPath": "/reports/xxx.pdf"
}
```

## 7. 治理与运维（管理员）

### `GET /api/v1/audits`（管理员/医生）

- 查询参数：`action`、`entityType`、`entityId`、`result`、`limit`

### `GET /api/v1/analytics/admin-overview`

- 返回：系统统计、最近用户、最近操作

### `GET /api/v1/ops/readiness`

- 返回：DB/AI 就绪 + 证据门禁

### `GET /api/v1/ops/dashboard`

- 返回：证据摘要、开放事件统计

### `GET /api/v1/ops/evidence`

- 查询参数：`limit`

### `POST /api/v1/ops/evidence`

- 请求体示例：

```json
{
  "runName": "val-run-2026-02-25",
  "modelVersion": "cxr-multitask-v1",
  "datasetName": "nih-val",
  "sampleCount": 2000,
  "positiveCount": 600,
  "siteCount": 3,
  "auroc": 0.92,
  "sensitivity": 0.91,
  "specificity": 0.86,
  "stageRecommendation": "PILOT_DECISION_SUPPORT",
  "regulatoryStatus": "APPROVED"
}
```

### `GET /api/v1/ops/incidents`

- 查询参数：`status`、`limit`

### `POST /api/v1/ops/incidents`

- 请求体：

```json
{
  "source": "AI_SERVICE",
  "severity": "P1",
  "title": "AI timeout spike",
  "detail": "latency elevated",
  "ownerUserId": "uuid"
}
```

### `POST /api/v1/ops/incidents/:id/transition`

- 请求体：

```json
{
  "action": "ACKNOWLEDGE"
}
```

- `action`：`ACKNOWLEDGE|RESOLVE|REOPEN`

## 8. 常见错误码

- `400`：请求参数/状态非法
- `401`：未登录或 token 无效
- `403`：角色权限不足
- `404`：资源不存在
- `429`：触发网关限流保护
- `500`：服务内部错误
- `502`：AI 服务不可用或返回异常
- `503`：下游熔断打开 / `ready` 检查未就绪
