package main

import (
	"log"
	"net/http"
	"os"
	"strings"

	"cancer-detection-backend/internal/platform/gormdb"

	"github.com/gin-gonic/gin"
)

func main() {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8083"
	}
	dsn := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if dsn == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if _, err := gormdb.Open(dsn); err != nil {
		log.Fatalf("init gorm: %v", err)
	}

	r := gin.Default()
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "governance-service"})
	})

	v1 := r.Group("/api/v1")
	{
		v1.GET("/audits", notImplemented("listAudits"))
		v1.GET("/analytics/admin-overview", notImplemented("adminOverview"))
		v1.GET("/ops/readiness", notImplemented("opsReadiness"))
		v1.GET("/ops/dashboard", notImplemented("opsDashboard"))
		v1.GET("/ops/evidence", notImplemented("listEvidence"))
		v1.POST("/ops/evidence", notImplemented("createEvidence"))
		v1.GET("/ops/incidents", notImplemented("listIncidents"))
		v1.POST("/ops/incidents", notImplemented("createIncident"))
		v1.POST("/ops/incidents/:id/transition", notImplemented("transitionIncident"))
	}

	log.Printf("governance-service listening on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal(err)
	}
}

func notImplemented(op string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusNotImplemented, gin.H{
			"error":     "service endpoint not implemented yet",
			"operation": op,
		})
	}
}
