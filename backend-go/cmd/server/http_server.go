package main

// File: cmd/server/http_server.go
// Purpose: Gateway handlers, middleware, and wiring for external HTTP APIs.

import (
	"context"
	"net/http"
	"strings"
	"time"

	"cancer-detection-backend/internal/config"
	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/platform/bootstrap"
	"cancer-detection-backend/internal/resilience"
	"cancer-detection-backend/internal/rpc/bridge"
	rpccodec "cancer-detection-backend/internal/rpc/codec"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func newApp(ctx context.Context, cfg *config.Config) (*app, error) {
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
	}
	if conn, err := dialGRPC(cfg.DetectionServiceGRPC); err == nil && conn != nil {
		instance.detectionGRPCConn = conn
		instance.detectionBridge = bridge.NewServiceClient(conn)
	}
	if conn, err := dialGRPC(cfg.ReportServiceGRPC); err == nil && conn != nil {
		instance.reportGRPCConn = conn
		instance.reportBridge = bridge.NewServiceClient(conn)
	}
	if conn, err := dialGRPC(cfg.GovernanceServiceGRPC); err == nil && conn != nil {
		instance.governanceGRPCConn = conn
		instance.governanceBridge = bridge.NewServiceClient(conn)
	}
	return instance, nil
}

func (a *app) close() {
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

func dialGRPC(addr string) (*grpc.ClientConn, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return nil, nil
	}
	return grpc.Dial(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(rpccodec.New())),
		grpc.WithUnaryInterceptor(observability.UnaryClientTraceInterceptor("api-gateway.grpc-client")),
	)
}
