package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/platform/gormdb"
	"cancer-detection-backend/internal/platform/lifecycle"
	"cancer-detection-backend/internal/repository"
	"cancer-detection-backend/internal/rpc/bridge"
	"cancer-detection-backend/internal/rpc/bridgehttp"
	rpccodec "cancer-detection-backend/internal/rpc/codec"
	"cancer-detection-backend/internal/service"

	"google.golang.org/grpc"
)

func main() {
	traceShutdown, err := observability.InitTracingFromEnv(context.Background(), "governance-service")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = traceShutdown(context.Background())
	}()

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8083"
	}
	dsn := resolveServiceDSN("GOVERNANCE_DATABASE_URL")
	if dsn == "" {
		log.Fatal("GOVERNANCE_DATABASE_URL is required")
	}
	jwtSecret := strings.TrimSpace(os.Getenv("BACKEND_JWT_SECRET"))
	if jwtSecret == "" {
		jwtSecret = "change-me-in-production"
	}

	gdb, err := gormdb.Open(dsn)
	if err != nil {
		log.Fatalf("init gorm: %v", err)
	}

	app := &governanceApp{
		auditSvc:      service.NewAuditService(repository.NewGormAuditRepository(gdb)),
		analyticsSvc:  service.NewAnalyticsService(repository.NewGormAnalyticsRepository(gdb)),
		opsSvc:        service.NewOpsService(repository.NewGormOpsRepository(gdb)),
		evidenceStore: repository.NewGormEvidenceStore(gdb),
		httpClient:    &http.Client{Timeout: 20 * time.Second},
		aiServiceURL:  strings.TrimRight(envOr("AI_SERVICE_URL", "http://localhost:8000"), "/"),
	}

	consumerCtx, consumerCancel := context.WithCancel(context.Background())
	defer consumerCancel()
	startEventConsumer(consumerCtx, app.auditSvc)

	router := buildGovernanceRouter(app, []byte(jwtSecret))

	grpcPort := strings.TrimSpace(os.Getenv("GRPC_PORT"))
	if grpcPort == "" {
		grpcPort = "9083"
	}
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("governance grpc listen error: %v", err)
	}
	grpcServer := grpc.NewServer(
		grpc.ForceServerCodec(rpccodec.New()),
		grpc.UnaryInterceptor(observability.UnaryServerTraceInterceptor("governance-service.grpc-server")),
	)
	bridge.RegisterServiceServer(grpcServer, bridgehttp.New(router))

	httpServer := &http.Server{Addr: ":" + port, Handler: router}
	if err := lifecycle.Run(context.Background(), lifecycle.Runtime{
		Name:            "governance-service",
		HTTPServer:      httpServer,
		GRPCServer:      grpcServer,
		GRPCListener:    lis,
		ShutdownTimeout: 10 * time.Second,
	}); err != nil {
		log.Fatal(err)
	}
}

func resolveServiceDSN(serviceEnv string) string {
	return strings.TrimSpace(os.Getenv(serviceEnv))
}
