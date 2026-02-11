# X-ray Cancer Detection System - Setup Instructions

## Documentation Languages

- English: `SETUP.md`
- 简体中文: `SETUP.zh-CN.md`

## Prerequisites

- Node.js 18+ installed
- Docker Desktop installed and running
- PostgreSQL (via Docker or local)

## Installation Steps

### 1. Install Dependencies
```bash
npm install
```

### 2. Start Infrastructure Services
```bash
# Start PostgreSQL, Redis and AI Service using Docker
docker-compose up -d

# Or run PostgreSQL/Redis/AI service locally
```

### 3. Setup Database
```bash
# Generate Prisma client
npx prisma generate

# Run database migrations
npx prisma migrate dev --name init

# (Optional) Seed database with sample data
npm run db:seed
```

### 4. Environment Variables
The `.env` file is already configured with default values:
- Database: PostgreSQL on localhost:5432
- Redis: localhost:6379
- AI service: localhost:8000
- NextAuth secret (change in production)

**Important:** Change `NEXTAUTH_SECRET` before deploying to production:
```bash
openssl rand -base64 32
```

### 5. Run Development Server
```bash
npm run dev
```

The application will be available at http://localhost:3000

## Real AI Detection Integration

### AI service endpoint
The Next.js upload API calls:
- `POST {AI_SERVICE_URL}/predict`
- `GET {AI_SERVICE_URL}/health`

### Expected `/predict` response
```json
{
  "modelVersion": "cxr-multitask-v1",
  "cancerProbability": 0.74,
  "regions": [],
  "labelScores": {
    "肺炎(Pneumonia)": 0.61,
    "结节(Nodule)": 0.55,
    "肿块(Mass)": 0.74,
    "浸润/实变(Opacity)": 0.58
  },
  "topFindings": ["肿块(Mass)", "肺炎(Pneumonia)", "浸润/实变(Opacity)"],
  "calibrationTemperature": 1.6,
  "decisionHighSensitivity": true,
  "decisionHighSpecificity": true,
  "clinicalUse": "RESEARCH_ONLY",
  "heatmapPath": null
}
```

### Use your own trained ONNX model
1. Put model file at:
`ai-service/models/cxr_multitask.onnx`
2. Restart ai-service:
```bash
docker-compose up -d --build ai-service
```
3. Check health:
```bash
curl http://localhost:8000/health
```

If no ONNX model is found, the AI service uses a deterministic image-analysis fallback (non-random) so the pipeline remains functional.

### Export multitask ONNX (A: Pneumonia + B: Nodule/Mass)
```bash
cd ai-service
python -m venv .venv_export
.venv_export\Scripts\activate
pip install -r requirements-export.txt
python scripts/export_torchxrayvision_multitask_to_onnx.py --output ../models/cxr_multitask.onnx
```

Output channel order of the exported model:
1. Pneumonia
2. Nodule
3. Mass
4. Lung Opacity

### Evaluate on your validation set
Prepare CSV with columns: `y_true,y_score`
```bash
python scripts/evaluate_predictions.py --csv .\your_val.csv --thr-sens 0.30 --thr-spec 0.70 --out .\eval_report.json
```

### Run AI service locally (without Docker)
If Docker image pulling is restricted in your environment:
```bash
cd ai-service
python -m venv .venv
.venv\Scripts\activate
pip install -r requirements.txt
uvicorn app.main:app --host 0.0.0.0 --port 8000
```

## Default User Accounts

You'll need to register users through the `/register` page.

### User Roles
- **PATIENT**: Can upload X-ray images and view results
- **DOCTOR**: Can review AI results, annotate images, generate reports
- **ADMIN**: System management and statistics

## Project Structure

```
cancer-detection-system/
├── app/                      # Next.js App Router pages
│   ├── (auth)/              # Authentication pages
│   ├── (dashboard)/         # Protected dashboard pages (to be implemented)
│   └── api/                 # API routes
├── components/              # React components
│   ├── ui/                  # Base UI components
│   ├── image-viewer/        # Image viewing components (to be implemented)
│   ├── annotation/          # Annotation tools (to be implemented)
│   └── charts/              # Chart components (to be implemented)
├── lib/                     # Utility functions
├── server/                  # tRPC server
│   ├── routers/             # tRPC routers
│   ├── services/            # Business logic (to be implemented)
│   └── ai/                  # AI model integration (to be implemented)
├── prisma/                  # Database schema
└── types/                   # TypeScript types
```

## Next Steps

### Phase 1: Basic Infrastructure ✅
- [x] Project initialization
- [x] Database setup
- [x] Authentication system
- [x] Base UI components

### Phase 2: Core Features (In Progress)
- [ ] Patient dashboard
- [ ] Doctor workstation
- [ ] Image upload functionality
- [ ] AI detection integration
- [ ] Report generation

### Phase 3: Advanced Features
- [ ] Admin panel
- [ ] Data analytics
- [ ] Performance optimization
- [ ] Security hardening

## Troubleshooting

### Docker not running
```bash
# Start Docker Desktop manually
# Then run: docker-compose up -d
```

### Database connection issues
```bash
# Check if PostgreSQL is running
docker ps

# View logs
docker logs cancer-detection-db
```

### Prisma Client errors
```bash
# Regenerate Prisma client
npx prisma generate

# Reset database (WARNING: deletes all data)
npx prisma migrate reset
```

## Development Commands

```bash
# Start development server
npm run dev

# Build for production
npm run build

# Start production server
npm start

# Run linter
npm run lint

# Format code
npm run format

# Database commands
npx prisma studio        # Open Prisma Studio
npx prisma migrate dev   # Create migration
npx prisma db push       # Push schema without migration
```

## Technology Stack

- **Framework**: Next.js 14 (App Router)
- **Language**: TypeScript
- **Styling**: Tailwind CSS
- **Database**: PostgreSQL + Prisma ORM
- **API**: tRPC
- **Authentication**: NextAuth.js
- **State Management**: React Query
- **Caching**: Redis

## Security Notes

- All passwords are hashed using bcrypt
- JWT tokens for session management
- HTTPS required in production
- CORS properly configured
- SQL injection prevention via Prisma
- XSS protection built-in

## Contributing

This is a medical application. Please ensure:
1. All code is thoroughly tested
2. Security best practices followed
3. HIPAA compliance maintained
4. Code review before merge
