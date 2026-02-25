package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *app) adminOverview(c *gin.Context) {
	claims := getClaims(c)
	if !a.assertAdmin(c, claims) {
		return
	}
	cacheKey := "analytics:admin_overview:v1"
	var cached map[string]any
	if a.cacheGet(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	out, err := a.analyticsService.AdminOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load analytics"})
		return
	}

	response := gin.H{
		"stats": gin.H{
			"users":             out.Users,
			"doctors":           out.Doctors,
			"patients":          out.Patients,
			"images":            out.Images,
			"detections":        out.Detections,
			"pendingDetections": out.PendingDetections,
			"reports":           out.Reports,
			"finalizedReports":  out.FinalizedReports,
		},
		"recentUsers":    out.RecentUsers,
		"recentActivity": out.RecentActivity,
	}
	a.cacheSet(c, cacheKey, response, 30*time.Second)
	c.JSON(http.StatusOK, response)
}
