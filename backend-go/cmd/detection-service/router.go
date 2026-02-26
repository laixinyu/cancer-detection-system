package main

import (
	"net/http"

	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
)

func buildDetectionRouter(detectionSvc *service.DetectionService, jwtSecret []byte) *gin.Engine {
	metrics := observability.NewRegistry()
	r := gin.Default()
	r.Use(observability.GinMetricsMiddleware(metrics))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "detection-service"})
	})
	r.GET("/metrics", observability.GinMetricsHandler(metrics))

	h := &detectionHandler{detectionSvc: detectionSvc}
	v1 := r.Group("/api/v1")
	v1.Use(authRequired(jwtSecret))
	{
		v1.GET("/detections", h.listDetections)
		v1.GET("/detections/:id", h.getDetection)
		v1.POST("/detections/:id/review", h.reviewDetection)
	}

	return r
}
