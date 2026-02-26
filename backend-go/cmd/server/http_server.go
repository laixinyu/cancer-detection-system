package main

// 文件： cmd/server/http_server.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"cancer-detection-backend/internal/config"
	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/events"
	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/platform/bootstrap"
	"cancer-detection-backend/internal/repository"
	"cancer-detection-backend/internal/resilience"
	"cancer-detection-backend/internal/rpc/bridge"
	rpccodec "cancer-detection-backend/internal/rpc/codec"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func newApp(ctx context.Context, cfg *config.Config) (*app, error) {
	if err := validateMicroserviceRoutingConfig(cfg); err != nil {
		return nil, err
	}

	deps, err := bootstrap.BuildGateway(ctx, cfg)
	if err != nil {
		return nil, err
	}

	instance := &app{
		db:                   deps.DB,
		orm:                  deps.ORM,
		evidenceStore:        deps.EvidenceStore,
		auditService:         deps.AuditService,
		authService:          deps.AuthService,
		detectionService:     deps.DetectionService,
		reportService:        deps.ReportService,
		analyticsService:     deps.AnalyticsService,
		opsService:           deps.OpsService,
		uploadService:        deps.UploadService,
		metrics:              deps.Metrics,
		logger:               deps.Logger,
		slowRequestThreshold: cfg.SlowRequestThreshold,
		cache:                deps.Cache,
		jwtSecret:            []byte(cfg.JWTSecret),
		aiService:            cfg.AIServiceURL,
		detectionServiceURL:  cfg.DetectionServiceURL,
		reportServiceURL:     cfg.ReportServiceURL,
		governanceServiceURL: cfg.GovernanceServiceURL,
		uploadDir:            cfg.UploadDir,
		uploadPrefix:         cfg.UploadPublicPrefix,
		httpClient:           deps.HTTPClient,
		inboundLimiter:       resilience.NewKeyedLimiter(cfg.InboundRateLimitRPS, cfg.InboundRateLimitBurst, 3*time.Minute),
		aiLimiter:            resilience.NewKeyedLimiter(cfg.AIOutboundRPS, cfg.AIOutboundBurst, 3*time.Minute),
		upstreamBreakers:     resilience.NewBreakerGroup(cfg.BreakerFailThreshold, cfg.BreakerOpenTimeout, cfg.BreakerHalfOpenCalls),
		tracer:               otel.Tracer("gateway.http"),
		strictMicroservice:   cfg.StrictMicroserviceMode(),
		idempotencyRepo:      repository.NewGormIdempotencyRepository(deps.ORM),
		outboxRepo:           repository.NewGormOutboxRepository(deps.ORM),
		idempotencyEnabled:   cfg.IdempotencyEnabled,
		idempotencyRequired:  cfg.IdempotencyRequired,
		idempotencyTTL:       cfg.IdempotencyTTL,
	}
	if err := deps.ORM.AutoMigrate(&domain.IdempotencyRecord{}, &domain.OutboxEvent{}); err != nil {
		return nil, fmt.Errorf("migrate gateway consistency tables: %w", err)
	}
	publisher, err := buildEventPublisher(cfg, deps.Logger)
	if err != nil {
		return nil, err
	}
	instance.eventPublisher = publisher
	if cfg.OutboxRelayEnabled && instance.outboxRepo != nil && instance.eventPublisher != nil {
		relayCtx, cancel := context.WithCancel(context.Background())
		instance.relayCancel = cancel
		relay := events.NewRelay(instance.outboxRepo, instance.eventPublisher, deps.Logger, cfg.OutboxRelayBatch, cfg.OutboxMaxAttempts, cfg.OutboxRelayInterval)
		go relay.Run(relayCtx)
	}
	if conn, err := dialGRPC(cfg, cfg.DetectionServiceGRPC); err == nil && conn != nil {
		instance.detectionGRPCConn = conn
		instance.detectionBridge = bridge.NewServiceClient(conn)
	} else if err != nil {
		deps.Logger.Warn("grpc detection bridge unavailable", "addr", cfg.DetectionServiceGRPC, "error", err.Error())
	}
	if conn, err := dialGRPC(cfg, cfg.ReportServiceGRPC); err == nil && conn != nil {
		instance.reportGRPCConn = conn
		instance.reportBridge = bridge.NewServiceClient(conn)
	} else if err != nil {
		deps.Logger.Warn("grpc report bridge unavailable", "addr", cfg.ReportServiceGRPC, "error", err.Error())
	}
	if conn, err := dialGRPC(cfg, cfg.GovernanceServiceGRPC); err == nil && conn != nil {
		instance.governanceGRPCConn = conn
		instance.governanceBridge = bridge.NewServiceClient(conn)
	} else if err != nil {
		deps.Logger.Warn("grpc governance bridge unavailable", "addr", cfg.GovernanceServiceGRPC, "error", err.Error())
	}
	return instance, nil
}

func validateMicroserviceRoutingConfig(cfg *config.Config) error {
	if !cfg.StrictMicroserviceMode() {
		return nil
	}

	if strings.TrimSpace(cfg.DetectionServiceGRPC) == "" && strings.TrimSpace(cfg.DetectionServiceURL) == "" {
		return fmt.Errorf("strict microservice mode requires DETECTION_SERVICE_GRPC_ADDR or DETECTION_SERVICE_URL")
	}
	if strings.TrimSpace(cfg.ReportServiceGRPC) == "" && strings.TrimSpace(cfg.ReportServiceURL) == "" {
		return fmt.Errorf("strict microservice mode requires REPORT_SERVICE_GRPC_ADDR or REPORT_SERVICE_URL")
	}
	if strings.TrimSpace(cfg.GovernanceServiceGRPC) == "" && strings.TrimSpace(cfg.GovernanceServiceURL) == "" {
		return fmt.Errorf("strict microservice mode requires GOVERNANCE_SERVICE_GRPC_ADDR or GOVERNANCE_SERVICE_URL")
	}
	return nil
}

func (a *app) close() {
	if a.relayCancel != nil {
		a.relayCancel()
	}
	if closer, ok := a.eventPublisher.(interface{ Close() error }); ok {
		_ = closer.Close()
	}
	bootstrap.CloseGateway(&bootstrap.GatewayDependencies{DB: a.db, Cache: a.cache})
	if a.inboundLimiter != nil {
		a.inboundLimiter.Close()
	}
	if a.aiLimiter != nil {
		a.aiLimiter.Close()
	}
	if a.detectionGRPCConn != nil {
		_ = a.detectionGRPCConn.Close()
	}
	if a.reportGRPCConn != nil {
		_ = a.reportGRPCConn.Close()
	}
	if a.governanceGRPCConn != nil {
		_ = a.governanceGRPCConn.Close()
	}
}

func buildEventPublisher(cfg *config.Config, logger *slog.Logger) (events.Publisher, error) {
	backend := strings.ToLower(strings.TrimSpace(cfg.EventBusBackend))
	switch backend {
	case "", "log", "none":
		return events.NewLogPublisher(logger), nil
	case "redis":
		pub, err := events.NewRedisStreamPublisher(cfg.EventBusRedisAddr, cfg.EventBusRedisPassword, cfg.EventBusRedisDB, cfg.EventBusRedisStream)
		if err != nil {
			if cfg.EventBusRequired {
				return nil, fmt.Errorf("init event bus redis: %w", err)
			}
			logger.Warn("event bus redis unavailable, fallback to log publisher", "error", err.Error())
			return events.NewLogPublisher(logger), nil
		}
		return pub, nil
	default:
		return nil, fmt.Errorf("unsupported EVENT_BUS_BACKEND: %s", cfg.EventBusBackend)
	}
}

func newRouter(a *app, cfg *config.Config) *gin.Engine {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(requestIDMiddleware())
	r.Use(a.traceMiddleware())
	r.Use(a.rateLimitMiddleware())
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
		authed.Use(a.idempotencyMiddleware())
		authed.GET("/images", a.listImages)
		authed.POST("/images/upload", a.uploadImage)
		authed.GET("/images/:id/file", a.imageFile)
		a.registerRestRoutes(authed)
	}
	return r
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

func dialGRPC(cfg *config.Config, addr string) (*grpc.ClientConn, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, nil
	}
	transportCreds, err := grpcTransportCredentials(cfg)
	if err != nil {
		return nil, err
	}
	return grpc.Dial(addr,
		grpc.WithTransportCredentials(transportCreds),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(rpccodec.New())),
		grpc.WithUnaryInterceptor(observability.UnaryClientTraceInterceptor("api-gateway.grpc-client")),
	)
}

func grpcTransportCredentials(cfg *config.Config) (credentials.TransportCredentials, error) {
	if cfg.GRPCInsecure {
		return insecure.NewCredentials(), nil
	}

	tlsCfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}
	if cfg.GRPCServerName != "" {
		tlsCfg.ServerName = cfg.GRPCServerName
	}
	if cfg.GRPCCACertFile != "" {
		pemData, err := os.ReadFile(cfg.GRPCCACertFile)
		if err != nil {
			return nil, fmt.Errorf("read grpc ca cert: %w", err)
		}
		roots := x509.NewCertPool()
		if !roots.AppendCertsFromPEM(pemData) {
			return nil, fmt.Errorf("append grpc ca cert failed")
		}
		tlsCfg.RootCAs = roots
	}
	return credentials.NewTLS(tlsCfg), nil
}
