# X 光肺部检测系统

面向胸部 X 光的 AI 辅助筛查平台，支持患者上传、医生审阅、报告生成与管理后台。

## 文档语言

- English: `README.md`
- 简体中文: `README.zh-CN.md`

## 当前状态

- 前后端：Next.js + tRPC + Prisma + PostgreSQL
- AI 服务：FastAPI + ONNX Runtime
- AI 任务：
  - A：肺炎风险
  - B：结节/肿块病灶风险
  - C：感染覆盖风险谱 + 白肺量化（筛查级）
- 核心筛查流程：
  - 上传时自动生成统一 `screeningSummary`（肺炎 + 病灶 + 白肺 + 分诊优先级）
  - 医生队列可按分诊优先级处理病例
  - 审阅页与报告页展示结构化筛查结果
- 合规与运维：
  - 默认关闭伪病灶框启发式输出
  - 默认 `RESEARCH_ONLY`（研究/辅助用途）
  - 支持高敏/高特双工作点
  - 已接入临床证据台账、治理门禁、运维事件管理

## 系统架构

```mermaid
flowchart TB
  U1[患者]
  U2[医生]
  U3[管理员]

  subgraph FE[前端层 Next.js App Router]
    P1[上传与患者页面]
    P2[医生审阅队列]
    P3[报告中心]
    P4[管理后台]
    P5[认证页面]
  end

  subgraph BE[应用层 Next.js API + tRPC]
    A1[NextAuth 认证与鉴权]
    A2[上传接口 /api/images/upload]
    A3[tRPC 路由 user/image/detection/report/compliance/audit/analytics/ops]
    A4[健康探针 /api/health /api/ready]
  end

  subgraph DB[数据层]
    D1[(PostgreSQL + Prisma)]
    D2[(Redis)]
  end

  subgraph AI[AI 推理层 FastAPI + ONNXRuntime]
    M1[多任务分类 肺炎 结节 肿块 浸润]
    M2[检测头 ONNX 病灶定位]
    M3[白肺量化与感染覆盖计算]
    M4[/health 与 /predict]
  end

  subgraph FS[存储层]
    F1[public/uploads 影像文件]
    F2[ai-service/models ONNX 模型与配置]
  end

  U1 --> FE
  U2 --> FE
  U3 --> FE

  FE --> BE
  A2 --> F1
  A2 --> M4
  M4 --> M1
  M4 --> M2
  M4 --> M3
  M1 --> F2
  M2 --> F2

  A3 --> D1
  A3 --> D2
  A1 --> D1
  A4 --> D1
  A4 --> M4
```

## 快速开始

### 1. 安装依赖

```bash
npm install
```

### 2. 启动基础服务与 AI 服务

```bash
docker compose up -d
```

### 3. 初始化数据库

```bash
npx prisma generate
npx prisma migrate dev --name init
```

### 4. 启动 Web 应用

```bash
npm run dev
```

访问地址：`http://localhost:3000`

## AI 模型流程

### 导出多任务 ONNX

```bash
cd ai-service
python -m venv .venv_export
.venv_export\Scripts\activate
pip install -r requirements-export.txt
python scripts/export_torchxrayvision_multitask_to_onnx.py --output ../models/cxr_multitask.onnx
```

导出通道顺序：
1. Pneumonia
2. Nodule
3. Mass
4. Lung Opacity

### 重建 AI 服务

```bash
cd ..
docker compose up -d --build ai-service
```

### 健康检查

```bash
Invoke-RestMethod http://localhost:8000/health
```

关键字段期望值：
- `modelLoaded: true`
- `modelPath: /app/models/cxr_multitask.onnx`
- `modelVersion: cxr-multitask-v1`
- `heuristicRegionsEnabled: false`

## 评估与校准

准备验证集 CSV 字段：
- `y_true`（0/1）
- `y_score`（0~1）

运行评估：

```bash
python ai-service/scripts/evaluate_predictions.py --csv .\your_val.csv --thr-sens 0.30 --thr-spec 0.70 --out .\eval_report.json
```

输出指标包括：
- AUROC
- ECE（10 bins）
- 高敏工作点指标
- 高特工作点指标

## 临床证据与运维就绪

- `clinical_evidence_runs`：记录验证数据集、样本量、多中心站点数、AUROC/敏感度/特异度、审批信息
- `ops_incidents`：记录 P0~P3 事件、状态流转、责任人
- 管理后台可查看：
  - 系统就绪状态（DB/AI）
  - 临床证据门禁是否通过
  - P0/P1 未关闭事件数

必要迁移命令：

```bash
npx prisma generate
npx prisma migrate dev --name add_clinical_evidence_and_ops
```

## 安全与访问规则

- 公共注册仅允许 `PATIENT`
- 患者仅可访问自己的影像/检测/报告
- 禁止硬删除影像（保障审计可追溯）
- 上传文件名由服务端生成
- 默认限制 AI 上传大小（10MB）
- 生产环境必须设置非默认 `NEXTAUTH_SECRET`

## 常用命令

```bash
npm run dev
npm run build
npm run lint
npx prisma studio
docker compose up -d
docker compose logs -f ai-service
```

## 关键路径

- `app/`：Next.js 页面与 API
- `server/`：tRPC 路由与服务端逻辑
- `prisma/schema.prisma`：数据库模型
- `ai-service/app/main.py`：AI 推理服务
- `ai-service/scripts/`：模型导出与评估脚本

## 说明

- 当前系统定位为研究/辅助筛查，不可替代临床最终诊断。
- 详细环境搭建与排障请查看 `SETUP.md` 或 `SETUP.zh-CN.md`。
