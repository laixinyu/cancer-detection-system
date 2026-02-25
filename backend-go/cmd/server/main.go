package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"cancer-detection-backend/internal/config"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	a, err := newApp(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer a.close()

	router := newRouter(a, cfg)
	srv := newHTTPServer(router, cfg)

	go func() {
		log.Printf("gin backend listening on %s", cfg.Addr())
		if serveErr := srv.ListenAndServe(); serveErr != nil && serveErr != http.ErrServerClosed {
			log.Fatalf("server error: %v", serveErr)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}
