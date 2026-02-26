package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func (h *governanceHandler) adminOverview(c *gin.Context) {
	if !assertAdmin(c, mustClaims(c)) {
		return
	}
	out, err := h.app.analyticsSvc.AdminOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load analytics"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
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
	})
}
