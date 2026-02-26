# Repository 边界审计

## Detection Domain

- 仓储：`internal/repository/detection.go`
- 允许表：`detections`, `images`, `patients`, `users`
- 禁止直接访问：`reports`, `audit_logs`, `ops_incidents`, `clinical_evidence_runs`

## Report Domain

- 仓储：`internal/repository/report.go`
- 允许表：`reports`, `detections`, `patients`, `users`, `images`
- 禁止直接访问：`audit_logs`, `ops_incidents`, `clinical_evidence_runs`

## Governance Domain

- 仓储：`internal/repository/audit.go`, `analytics.go`, `ops.go`, `evidence.go`
- 允许表：`audit_logs`, `ops_incidents`, `clinical_evidence_runs` 以及 analytics 只读聚合
- 禁止写入：`detections`, `reports`, `images`

## Auth/Upload Domain

- 仓储：`auth.go`, `upload.go`
- 允许表：`users`, `patients`, `images`, `detections`, `patient_consents`

## 规则

1. 新增仓储必须登记所属域与允许表。
2. 跨域数据获取优先走 API/事件，不允许跨域写入。
