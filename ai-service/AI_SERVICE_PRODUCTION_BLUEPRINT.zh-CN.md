# AI 服务生产蓝图

## 目标

1. 构建具备生产安全性的推理服务，并明确区分存活与就绪语义。
2. 让模型发布具备可审计性（治理门禁 + 分阶段发布）。
3. 提升延迟稳定性，并在流量突发时保护服务。
4. 建立可量化的 SLO 运维与故障响应机制。

## 当前关键风险（已审阅）

1. 服务逻辑长期集中在单文件，维护和安全演进成本高。
2. 运行时缺少完整生产护栏（并发/排队/超时/指标面）。
3. 健康检查语义在部分故障场景下不能准确表达“可推理”。
4. 部署基线缺少容器加固与显式运行默认值。

## 已落地改造

1. 运行时安全：
增加 `AI_MAX_CONCURRENT_REQUESTS`、推理超时、排队超时、在途请求上限。
2. 可观测性：
新增 `/metrics`，包含请求量、延迟、在途数、推理耗时指标。
3. 探针语义：
新增 `/livez`（存活），`/health`（就绪 + 治理阻断原因）。
4. 安全与治理：
支持 API Key 门禁，且在预测入口强制治理门禁。
5. 容器基线：
非 root 运行、健康检查、镜像内生产默认参数。

## 目标部署架构

1. 推理平面：
每个 Pod 使用单 worker FastAPI 进程（避免模型重复加载/内存争用）。
2. 路由平面：
网关负责认证/JWT + 幂等 + 追踪，AI 服务专注推理。
3. 模型制品平面：
模型仓库使用版本化对象存储 + 不可变 digest（sha256）+ 签名元数据。
4. 控制平面：
由治理服务驱动阶段：`RESEARCH_ONLY` -> `PILOT_DECISION_SUPPORT` -> `CLINICAL_DECISION_SUPPORT`。

## 模型部署策略

1. 制品契约：
`model.onnx`、`detector.onnx`、`manifest.json`、`governance.json`、`clinical_config.json`。
2. 推广流水线：
`train -> evaluate -> gate-check -> register -> canary -> full rollout`。
3. 发布策略：
Canary 5%（30 分钟）-> 25% -> 100%，前提是 SLO 与质量 KPI 全部通过。
4. 回滚策略：
当 p95 延迟、5xx、质量哨兵指标越界时，立即回滚到上一模型 digest。

## 推理服务重构计划（下一阶段）

1. 模块拆分：
`config.py`、`model_runtime.py`、`preprocess.py`、`postprocess.py`、`api.py`、`metrics.py`。
2. 类型化配置：
将直接读取环境变量改为统一配置模型并做校验。
3. 测试分层：
单测（前/后处理）、集成测试（/predict 契约）、负载冒烟测试。
4. 离线/在线一致性：
训练导出与在线推理复用同一预处理组件。

## SLO 与监控

1. 可用性：
`/predict` 月度 99.9%。
2. 延迟：
CPU 基线 p95 < 1.5s，p99 < 3s（GPU 独立压测与目标）。
3. 正确性护栏：
监控分数分布漂移、类别级阈值告警、detector 启动检查状态。
4. 告警建议：
5xx 率、队列饱和、超时率、治理阻断激增、模型重载失败。

## 立即执行清单

1. 生产环境变量：
`AI_REQUIRE_API_KEY=true`、`AI_API_KEY=<secret>`、`AI_ENFORCE_GOVERNANCE_GATE=true`。
2. 并发与超时调优：
先以 `AI_MAX_CONCURRENT_REQUESTS=2` 起步，再依据压测结果调参。
3. 监控看板：
接入请求延迟/错误、推理耗时直方图、在途数、就绪状态。
4. Canary Runbook：
明确审批人、KPI 阈值、回滚命令、对外沟通流程。
