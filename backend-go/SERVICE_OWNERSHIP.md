# 服务所有权矩阵

## ownership

1. `api-gateway`
- Owner: Platform Backend
- 职责: 认证、上传、文件访问、跨域编排、策略中间件
- 依赖: detection/report/governance、redis、postgres

2. `detection-service`
- Owner: AI Clinical Runtime
- 职责: 检测查询与复核域
- 数据边界: `DETECTION_DATABASE_URL`

3. `report-service`
- Owner: Clinical Reporting
- 职责: 报告查询/创建/更新域
- 数据边界: `REPORT_DATABASE_URL`

4. `governance-service`
- Owner: Safety & Ops
- 职责: 审计、分析、运维治理
- 数据边界: `GOVERNANCE_DATABASE_URL`

## 变更影响评估

每次改动需给出：

1. 影响服务列表
2. API 兼容性（向后兼容/不兼容）
3. 数据迁移风险（有/无）
4. 回滚方案
