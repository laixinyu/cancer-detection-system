package main

// 文件： cmd/detection-service/main.go
// 用途：检测微服务入口与启动流程。

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
	traceShutdown, err := observability.InitTracingFromEnv(context.Background(), "detection-service")
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = traceShutdown(context.Background())
	}()

	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8081"
	}
	dsn := resolveServiceDSN("DETECTION_DATABASE_URL")
	if dsn == "" {
		log.Fatal("DETECTION_DATABASE_URL is required")
	}
	jwtSecret := strings.TrimSpace(os.Getenv("BACKEND_JWT_SECRET"))
	if jwtSecret == "" {
		jwtSecret = "change-me-in-production"
	}

	gdb, err := gormdb.Open(dsn)
	if err != nil {
		log.Fatalf("init gorm: %v", err)
	}
	detectionSvc := service.NewDetectionService(repository.NewGormDetectionRepository(gdb))
	router := buildDetectionRouter(detectionSvc, []byte(jwtSecret))

	grpcPort := strings.TrimSpace(os.Getenv("GRPC_PORT"))
	if grpcPort == "" {
		grpcPort = "9081"
	}
	lis, err := net.Listen("tcp", ":"+grpcPort)
	if err != nil {
		log.Fatalf("detection grpc listen error: %v", err)
	}
	grpcServer := grpc.NewServer(
		grpc.ForceServerCodec(rpccodec.New()),
		grpc.UnaryInterceptor(observability.UnaryServerTraceInterceptor("detection-service.grpc-server")),
	)
	bridge.RegisterServiceServer(grpcServer, bridgehttp.New(router))

	httpServer := &http.Server{Addr: ":" + port, Handler: router}
	if err := lifecycle.Run(context.Background(), lifecycle.Runtime{
		Name:            "detection-service",
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
