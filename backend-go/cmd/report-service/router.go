package main

import (
	"net/http"

	"cancer-detection-backend/internal/observability"
	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
)

func buildReportRouter(reportSvc *service.ReportService, jwtSecret []byte) *gin.Engine {
	metrics := observability.NewRegistry()
	r := gin.Default()
	r.Use(observability.GinMetricsMiddleware(metrics))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "report-service"})
	})
	r.GET("/metrics", observability.GinMetricsHandler(metrics))

	h := &reportHandler{reportSvc: reportSvc}
	v1 := r.Group("/api/v1")
	v1.Use(authRequired(jwtSecret))
	{
		v1.GET("/reports", h.listReports)
		v1.GET("/reports/:id", h.getReport)
		v1.POST("/reports", h.createReport)
		v1.PATCH("/reports/:id", h.updateReport)
	}

	return r
}
