package main

import (
	"net/http"

	"cancer-detection-backend/internal/observability"

	"github.com/gin-gonic/gin"
)

func buildGovernanceRouter(app *governanceApp, jwtSecret []byte) *gin.Engine {
	metrics := observability.NewRegistry()
	r := gin.Default()
	r.Use(observability.GinMetricsMiddleware(metrics))
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "governance-service"})
	})
	r.GET("/metrics", observability.GinMetricsHandler(metrics))

	h := &governanceHandler{app: app}
	v1 := r.Group("/api/v1")
	v1.Use(authRequired(jwtSecret))
	{
		v1.GET("/audits", h.listAudits)
		v1.GET("/analytics/admin-overview", h.adminOverview)
		v1.GET("/ops/readiness", h.readiness)
		v1.GET("/ops/dashboard", h.dashboard)
		v1.GET("/ops/evidence", h.listEvidence)
		v1.POST("/ops/evidence", h.createEvidence)
		v1.GET("/ops/incidents", h.listIncidents)
		v1.POST("/ops/incidents", h.createIncident)
		v1.POST("/ops/incidents/:id/transition", h.transitionIncident)
	}

	return r
}

type governanceHandler struct {
	app *governanceApp
}
