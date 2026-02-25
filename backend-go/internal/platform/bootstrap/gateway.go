package bootstrap

// 文件： internal/platform/bootstrap/gateway.go
// 用途：组合根，装配网关基础设施、仓储与服务依赖。

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"cancer-detection-backend/internal/cache"
	"cancer-detection-backend/internal/config"
	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/platform/gormdb"
	"cancer-detection-backend/internal/repository"
	"cancer-detection-backend/internal/service"

	"github.com/jackc/pgx/v5/pgxpool"
	"gorm.io/gorm"
)

type GatewayDependencies struct {
	DB               *pgxpool.Pool
	ORM              *gorm.DB
	Cache            cache.Cache
	EvidenceStore    repository.EvidenceStore
	AuditService     *service.AuditService
	AuthService      *service.AuthService
	DetectionService *service.DetectionService
	ReportService    *service.ReportService
	AnalyticsService *service.AnalyticsService
	OpsService       *service.OpsService
	UploadService    *service.UploadService
	Metrics          *observability.Registry
	Logger           *slog.Logger
	HTTPClient       *http.Client
}

func BuildGateway(ctx context.Context, cfg *config.Config) (*GatewayDependencies, error) {
	db, err := pgxpool.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return nil, fmt.Errorf("connect db: %w", err)
	}
	gdb, err := gormdb.Open(cfg.DatabaseURL)
	if err != nil {
		db.Close()
		return nil, err
	}
	if err := os.MkdirAll(cfg.UploadDir, 0o755); err != nil {
		db.Close()
		return nil, fmt.Errorf("init upload dir: %w", err)
	}

	var appCache cache.Cache
	switch strings.ToLower(strings.TrimSpace(cfg.CacheBackend)) {
	case "none", "noop":
		appCache = cache.NewNoop()
	case "redis":
		rc, cacheErr := cache.NewRedis(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
		if cacheErr != nil {
			db.Close()
			return nil, fmt.Errorf("init redis cache: %w", cacheErr)
		}
		appCache = rc
	case "memory":
		appCache = cache.NewMemory()
	default:
		db.Close()
		return nil, fmt.Errorf("unsupported CACHE_BACKEND: %s", cfg.CacheBackend)
	}

	opsSvc := service.NewOpsService(repository.NewGormOpsRepository(gdb))
	deps := &GatewayDependencies{
		DB:               db,
		ORM:              gdb,
		Cache:            appCache,
		EvidenceStore:    repository.NewGormEvidenceStore(gdb),
		AuditService:     service.NewAuditService(repository.NewGormAuditRepository(gdb)),
		AuthService:      service.NewAuthService(repository.NewGormAuthRepository(gdb)),
		DetectionService: service.NewDetectionService(repository.NewGormDetectionRepository(gdb)),
		ReportService:    service.NewReportService(repository.NewGormReportRepository(gdb)),
		AnalyticsService: service.NewAnalyticsService(repository.NewGormAnalyticsRepository(gdb)),
		OpsService:       opsSvc,
		UploadService:    service.NewUploadService(repository.NewGormUploadRepository(gdb), opsSvc),
		Metrics:          observability.NewRegistry(),
		Logger:           buildLogger(cfg.LogLevel),
		HTTPClient:       &http.Client{Timeout: 25 * time.Second},
	}
	return deps, nil
}

func CloseGateway(deps *GatewayDependencies) {
	if deps == nil {
		return
	}
	if closer, ok := deps.Cache.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	if deps.DB != nil {
		deps.DB.Close()
	}
}

func buildLogger(level string) *slog.Logger {
	lvl := new(slog.LevelVar)
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl.Set(slog.LevelDebug)
	case "warn":
		lvl.Set(slog.LevelWarn)
	case "error":
		lvl.Set(slog.LevelError)
	default:
		lvl.Set(slog.LevelInfo)
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: lvl}))
}
