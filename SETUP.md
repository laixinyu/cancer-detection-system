# X 光肺部检测系统 - 环境搭建指南

## 文档语言

- 当前文档：`SETUP.md`（中文）
- 中文副本：`SETUP.zh-CN.md`

## 前置条件

- 已安装 Node.js 18+
- 已安装并启动 Docker Desktop
- PostgreSQL（可用 Docker 或本地）

## 安装步骤

### 1. 安装依赖

```bash
npm install
```

### 2. 启动基础服务

```bash
# 使用 Docker 启动 PostgreSQL、Redis、AI 服务
docker compose up -d
```

### 3. 初始化数据库

```bash
# 首次启动会执行 deploy/postgres/init/*.sql（创建四个服务数据库与最小权限账号）
# 如需重置数据库初始化（会清空数据）：
docker compose down -v
docker compose up -d
```

### 4. 环境变量（关键）

项目内 `.env` 默认已配置：
- 网关与三服务独立 DSN：
  - `GATEWAY_DATABASE_URL`
  - `DETECTION_DATABASE_URL`
  - `REPORT_DATABASE_URL`
  - `GOVERNANCE_DATABASE_URL`
- Redis：`REDIS_ADDR`
- AI 服务：`AI_SERVICE_URL`
- 对象存储（S3/MinIO）：
  - `STORAGE_BACKEND=s3`
  - `S3_ENDPOINT`
  - `S3_REGION`
  - `S3_BUCKET`
  - `S3_ACCESS_KEY_ID`
  - `S3_SECRET_ACCESS_KEY`
  - `S3_USE_PATH_STYLE`
  - `S3_USE_TLS`
  - `UPLOAD_PUBLIC_PREFIX`
- NextAuth 密钥（生产环境必须替换）

生产环境请先替换密钥：

```bash
openssl rand -base64 32
```

### 5. 对象存储（S3/MinIO）

当前上传链路已统一为 S3 兼容对象存储，不再依赖本地 `public/uploads`。

MinIO 本地开发推荐：

```env
STORAGE_BACKEND=s3
S3_ENDPOINT=http://127.0.0.1:9000
S3_REGION=us-east-1
S3_BUCKET=cancer-images
S3_ACCESS_KEY_ID=minioadmin
S3_SECRET_ACCESS_KEY=minioadmin
S3_USE_PATH_STYLE=true
S3_USE_TLS=false
UPLOAD_PUBLIC_PREFIX=/uploads
```

说明：
- `S3_USE_PATH_STYLE=true` 对 MinIO 通常是必需的。
- 影像对外访问仍通过 `/api/v1/images/:id/file`，不会直接暴露桶地址。

### 6. 启动开发服务

```bash
npm run dev
```

访问：`http://localhost:3000`

## 真实 AI 检测接入

### Next.js 调用的 AI 接口

- `POST {AI_SERVICE_URL}/predict`
- `GET {AI_SERVICE_URL}/health`

### `/predict` 关键输出（示例）

```json
{
  "modelVersion": "cxr-multitask-v1",
  "cancerProbability": 0.74,
  "labelScores": {
    "肺炎(Pneumonia)": 0.61,
    "结节(Nodule)": 0.55,
    "肿块(Mass)": 0.74,
    "浸润/实变(Opacity)": 0.58
  },
  "infectionCoverage": {
    "infectionAny": 0.66,
    "covidLikeWhiteLungPattern": 0.42
  },
  "whiteLungAssessment": {
    "whiteLungScore": 0.31,
    "severity": "mild"
  }
}
```

### 使用你自己的 ONNX 模型

1. 模型放置到：`ai-service/models/cxr_multitask.onnx`
2. 重启 AI 服务：

```bash
docker compose up -d --build ai-service
```

3. 健康检查：

```bash
curl http://localhost:8000/health
```

## 导出多任务 ONNX（肺炎 + 结节/肿块）

```bash
cd ai-service
python -m venv .venv_export
.venv_export\Scripts\activate
pip install -r requirements-export.txt
python scripts/export_torchxrayvision_multitask_to_onnx.py --output ../models/cxr_multitask.onnx
```

## 在验证集上评估

准备 CSV（`y_true,y_score`）：

```bash
python scripts/evaluate_predictions.py --csv .\your_val.csv --thr-sens 0.30 --thr-spec 0.70 --out .\eval_report.json
```

推荐使用完整流水线（自动导出验证集预测并生成临床阈值配置）：

```bash
npm run ai:validate -- --dataset-root "E:\datasets\ChestXray-NIHCC-512" --checkpoint "ai-service/models/nih_multitask_efficientnet_v2_s_best.pt" --target-split val
```

## 无法使用 Docker 时的本地运行

```bash
cd ai-service
python -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt
uvicorn app.main:app --host 0.0.0.0 --port 8000
```

## 角色说明

- `PATIENT`：上传影像、查看结果与报告
- `DOCTOR`：审阅 AI 结果、标注、生成报告
- `ADMIN`：管理系统、查看统计与运维状态

## 排障

### Docker 未运行

```bash
docker compose up -d
```

### 数据库连接失败

```bash
docker ps
docker logs cancer-detection-db
```

### 对象存储读取失败（`/api/v1/images/:id/file`）

```bash
docker logs cancer-detection-backend-go
docker logs cancer-detection-minio
```

检查：
- `S3_ENDPOINT/S3_BUCKET/S3_ACCESS_KEY_ID/S3_SECRET_ACCESS_KEY` 是否一致
- `S3_USE_PATH_STYLE` 在 MinIO 下是否为 `true`

## 常用命令

```bash
npm run dev
npm run build
npm start
npm run lint
npm run check:arch
docker compose up -d
docker compose logs -f cancer-detection-backend-go
```

## 技术栈

- 框架：Next.js（App Router）
- 语言：TypeScript
- 样式：Tailwind CSS
- 后端：Go Gin API Gateway + detection/report/governance 微服务
- 数据库：PostgreSQL（服务级独立 DSN）
- 缓存与事件：Redis（Cache + Streams）
- 文件存储：S3/MinIO 对象存储
- 认证：NextAuth.js
- 推理服务：FastAPI + ONNX Runtime

## 安全提醒

- 密码使用 bcrypt 哈希存储
- 生产环境必须启用 HTTPS
- 服务端参数化查询与输入校验防注入
- 当前系统为辅助筛查，不可直接用于临床确诊
