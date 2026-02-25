# X 光肺部检测系统 - 环境搭建指南

## 文档语言

- 当前文档：`SETUP.zh-CN.md`（中文）
- 中文主文档：`SETUP.md`

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
# 生成 Prisma Client
npx prisma generate

# 执行迁移
npx prisma migrate dev --name init
```

### 4. 环境变量

项目内 `.env` 默认已配置：
- 数据库：PostgreSQL（localhost:5432）
- Redis：localhost:6379
- AI 服务：localhost:8000
- NextAuth 密钥（生产环境必须替换）

生产环境请先替换密钥：

```bash
openssl rand -base64 32
```

### 可选：上传存储切换（本地 / MinIO / S3）

默认 local 模式为本地对象存储（MinIO，S3 兼容）：

```env
STORAGE_PROVIDER=local
STORAGE_ENV=dev
STORAGE_ENV_IN_PREFIX=true
S3_ENDPOINT=http://127.0.0.1:9000
S3_REGION=us-east-1
S3_BUCKET=cancer-images-local
S3_ACCESS_KEY_ID=minioadmin
S3_SECRET_ACCESS_KEY=minioadmin
S3_FORCE_PATH_STYLE=true
S3_PREFIX=uploads
```

环境隔离可选配置：

```env
S3_BUCKET_DEV=cancer-images-dev
S3_BUCKET_TEST=cancer-images-test
S3_BUCKET_PROD=cancer-images-prod
S3_PREFIX_DEV=uploads
S3_PREFIX_TEST=uploads
S3_PREFIX_PROD=uploads
```

切换到 MinIO / S3：

```env
STORAGE_PROVIDER=s3
S3_ENDPOINT=http://127.0.0.1:9000
S3_REGION=us-east-1
S3_BUCKET=cancer-images
S3_ACCESS_KEY_ID=minioadmin
S3_SECRET_ACCESS_KEY=minioadmin
S3_FORCE_PATH_STYLE=true
S3_PREFIX=uploads
```

说明：
- `S3_ENDPOINT` 留空时即使用 AWS S3 官方端点。
- MinIO 通常需要 `S3_FORCE_PATH_STYLE=true`。
- 系统会把数据库中的 `file_path` 存成 `s3://bucket/key`，并通过受控接口返回可访问地址。
- 如需强制回退到磁盘存储，可设置：
  `STORAGE_PROVIDER=fs`、`LOCAL_UPLOAD_DIR=./public/uploads`、`LOCAL_UPLOAD_PUBLIC_PREFIX=/uploads`。

### 5. 启动开发服务

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

### Prisma 相关报错

```bash
npx prisma generate
npx prisma migrate reset
```

## 常用命令

```bash
npm run dev
npm run build
npm start
npm run lint
npx prisma studio
npx prisma migrate dev
npx prisma db push
```

## 技术栈

- 框架：Next.js（App Router）
- 语言：TypeScript
- 样式：Tailwind CSS
- 数据库：PostgreSQL + Prisma
- 接口层：tRPC
- 认证：NextAuth.js
- 推理服务：FastAPI + ONNX Runtime

## 安全提醒

- 密码使用 bcrypt 哈希存储
- 生产环境必须启用 HTTPS
- 使用 Prisma 避免 SQL 注入
- 当前系统为辅助筛查，不可直接用于临床确诊
