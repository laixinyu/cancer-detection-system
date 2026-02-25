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
		port = "8081"
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
		c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "detection-service"})
	})

	v1 := r.Group("/api/v1")
	{
		v1.GET("/detections", notImplemented("listDetections"))
		v1.GET("/detections/:id", notImplemented("getDetectionByID"))
		v1.POST("/detections/:id/review", notImplemented("reviewDetection"))
	}

	log.Printf("detection-service listening on :%s", port)
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
