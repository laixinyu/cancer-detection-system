package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"cancer-detection-backend/internal/cache"
	"cancer-detection-backend/internal/config"
	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/platform/gormdb"
	"cancer-detection-backend/internal/repository"
	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
)

func newApp(ctx context.Context, cfg *config.Config) (*app, error) {
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
	switch cfg.CacheBackend {
	case "none", "noop":
		appCache = cache.NewNoop()
	default:
		appCache = cache.NewMemory()
	}

	instance := &app{
		db:                   db,
		orm:                  gdb,
		evidenceStore:        repository.NewGormEvidenceStore(gdb),
		auditService:         service.NewAuditService(repository.NewGormAuditRepository(gdb)),
		authService:          service.NewAuthService(repository.NewGormAuthRepository(gdb)),
		detectionService:     service.NewDetectionService(repository.NewGormDetectionRepository(gdb)),
		reportService:        service.NewReportService(repository.NewGormReportRepository(gdb)),
		analyticsService:     service.NewAnalyticsService(repository.NewGormAnalyticsRepository(gdb)),
		opsService:           service.NewOpsService(repository.NewGormOpsRepository(gdb)),
		uploadService:        nil,
		metrics:              observability.NewRegistry(),
		logger:               buildLogger(cfg.LogLevel),
		slowRequestThreshold: cfg.SlowRequestThreshold,
		cache:                appCache,
		jwtSecret:            []byte(cfg.JWTSecret),
		aiService:            cfg.AIServiceURL,
		detectionServiceURL:  cfg.DetectionServiceURL,
		reportServiceURL:     cfg.ReportServiceURL,
		governanceServiceURL: cfg.GovernanceServiceURL,
		uploadDir:            cfg.UploadDir,
		uploadPrefix:         cfg.UploadPublicPrefix,
		httpClient:           &http.Client{Timeout: 25 * time.Second},
	}
	instance.uploadService = service.NewUploadService(repository.NewGormUploadRepository(gdb), instance.opsService)
	return instance, nil
}

func (a *app) close() {
	if closer, ok := a.cache.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	if a.db != nil {
		a.db.Close()
	}
}

func newRouter(a *app, cfg *config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestIDMiddleware())
	r.Use(a.accessLogMiddleware())
	r.Use(corsMiddleware())
	r.Use(timeoutMiddleware(cfg.RequestTimeout))
	r.MaxMultipartMemory = 12 << 20

	r.GET("/health", a.health)
	r.GET("/ready", a.ready)
	r.GET("/metrics", a.metricsHandler)

	v1 := r.Group("/api/v1")
	{
		v1.POST("/auth/register", a.register)
		v1.POST("/auth/login", a.login)

		authed := v1.Group("")
		authed.Use(a.authRequired())
		authed.GET("/images", a.listImages)
		authed.POST("/images/upload", a.uploadImage)
		authed.GET("/images/:id/file", a.imageFile)
		a.registerRestRoutes(authed)
	}
	return r
}

func buildLogger(level string) *slog.Logger {
	lvl := new(slog.LevelVar)
	switch level {
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

func newHTTPServer(handler http.Handler, cfg *config.Config) *http.Server {
	return &http.Server{
		Addr:         cfg.Addr(),
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}
}
