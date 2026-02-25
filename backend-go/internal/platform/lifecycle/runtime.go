package lifecycle

// File: internal/platform/lifecycle/runtime.go
// Purpose: Unified service runtime lifecycle for HTTP/gRPC startup and graceful shutdown.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
)

type Runtime struct {
	Name            string
	HTTPServer      *http.Server
	GRPCServer      *grpc.Server
	GRPCListener    net.Listener
	ShutdownTimeout time.Duration
}

func Run(ctx context.Context, rt Runtime) error {
	if rt.HTTPServer == nil {
		return fmt.Errorf("http server is required")
	}
	if rt.ShutdownTimeout <= 0 {
		rt.ShutdownTimeout = 10 * time.Second
	}

	errCh := make(chan error, 2)
	go func() {
		log.Printf("%s http listening on %s", rt.Name, rt.HTTPServer.Addr)
		if err := rt.HTTPServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("http serve: %w", err)
		}
	}()

	if rt.GRPCServer != nil && rt.GRPCListener != nil {
		go func() {
			log.Printf("%s grpc listening on %s", rt.Name, rt.GRPCListener.Addr().String())
			if err := rt.GRPCServer.Serve(rt.GRPCListener); err != nil {
				errCh <- fmt.Errorf("grpc serve: %w", err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case <-ctx.Done():
	case <-stop:
	case err := <-errCh:
		return err
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), rt.ShutdownTimeout)
	defer cancel()
	if err := rt.HTTPServer.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("http shutdown: %w", err)
	}
	if rt.GRPCServer != nil {
		done := make(chan struct{})
		go func() {
			rt.GRPCServer.GracefulStop()
			close(done)
		}()
		select {
		case <-done:
		case <-shutdownCtx.Done():
			rt.GRPCServer.Stop()
		}
	}
	return nil
}
