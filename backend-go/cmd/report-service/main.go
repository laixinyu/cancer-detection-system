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
		port = "8082"
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
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "report-service"})
	})

	v1 := r.Group("/api/v1")
	{
		v1.GET("/reports", notImplemented("listReports"))
		v1.GET("/reports/:id", notImplemented("getReportByID"))
		v1.POST("/reports", notImplemented("createReport"))
		v1.PATCH("/reports/:id", notImplemented("updateReport"))
	}

	log.Printf("report-service listening on :%s", port)
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
