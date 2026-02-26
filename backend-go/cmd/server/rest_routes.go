package main

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *app) registerRestRoutes(authed *gin.RouterGroup) {
	detectionListTTL := 15 * time.Second
	reportListTTL := 15 * time.Second
	auditListTTL := 15 * time.Second
	analyticsOverviewTTL := 30 * time.Second
	opsDashboardTTL := 20 * time.Second
	opsEvidenceTTL := 30 * time.Second
	opsIncidentsTTL := 15 * time.Second

	detectionWriteInvalidations := []string{"detection:list:", "report:list:", "audit:list:", "analytics:", "ops:dashboard:"}
	reportWriteInvalidations := []string{"report:list:", "analytics:", "audit:list:"}
	opsEvidenceWriteInvalidations := []string{"ops:evidence:", "ops:dashboard:"}
	opsIncidentWriteInvalidations := []string{"ops:incidents:", "ops:dashboard:"}

	if a.detectionBridge != nil {
		authed.GET("/detections", a.cacheJSONRoute("detection:list:", detectionListTTL, a.grpcBridgeHandler(a.detectionBridge)))
		authed.GET("/detections/:id", a.grpcBridgeHandler(a.detectionBridge))
		authed.POST("/detections/:id/review", a.invalidateCacheOnSuccess(detectionWriteInvalidations, a.grpcBridgeHandler(a.detectionBridge)))
	} else if a.detectionServiceURL != "" {
		proxy := a.proxyTo(a.detectionServiceURL)
		authed.GET("/detections", a.cacheJSONRoute("detection:list:", detectionListTTL, proxy))
		authed.GET("/detections/:id", proxy)
		authed.POST("/detections/:id/review", a.invalidateCacheOnSuccess(detectionWriteInvalidations, proxy))
	} else {
		if a.strictMicroservice {
			unavailable := serviceUnavailableHandler("detection-service upstream is not configured")
			authed.GET("/detections", unavailable)
			authed.GET("/detections/:id", unavailable)
			authed.POST("/detections/:id/review", unavailable)
		} else {
			authed.GET("/detections", a.listDetections)
			authed.GET("/detections/:id", a.getDetectionByID)
			authed.POST("/detections/:id/review", a.reviewDetection)
		}
	}

	if a.reportBridge != nil {
		authed.GET("/reports", a.cacheJSONRoute("report:list:", reportListTTL, a.grpcBridgeHandler(a.reportBridge)))
		authed.GET("/reports/:id", a.grpcBridgeHandler(a.reportBridge))
		authed.POST("/reports", a.invalidateCacheOnSuccess(reportWriteInvalidations, a.grpcBridgeHandler(a.reportBridge)))
		authed.PATCH("/reports/:id", a.invalidateCacheOnSuccess(reportWriteInvalidations, a.grpcBridgeHandler(a.reportBridge)))
	} else if a.reportServiceURL != "" {
		proxy := a.proxyTo(a.reportServiceURL)
		authed.GET("/reports", a.cacheJSONRoute("report:list:", reportListTTL, proxy))
		authed.GET("/reports/:id", proxy)
		authed.POST("/reports", a.invalidateCacheOnSuccess(reportWriteInvalidations, proxy))
		authed.PATCH("/reports/:id", a.invalidateCacheOnSuccess(reportWriteInvalidations, proxy))
	} else {
		if a.strictMicroservice {
			unavailable := serviceUnavailableHandler("report-service upstream is not configured")
			authed.GET("/reports", unavailable)
			authed.GET("/reports/:id", unavailable)
			authed.POST("/reports", unavailable)
			authed.PATCH("/reports/:id", unavailable)
		} else {
			authed.GET("/reports", a.listReports)
			authed.GET("/reports/:id", a.getReportByID)
			authed.POST("/reports", a.createReport)
			authed.PATCH("/reports/:id", a.updateReport)
		}
	}

	if a.governanceBridge != nil {
		authed.GET("/audits", a.cacheJSONRoute("audit:list:", auditListTTL, a.grpcBridgeHandler(a.governanceBridge)))
		authed.GET("/analytics/admin-overview", a.cacheJSONRoute("analytics:", analyticsOverviewTTL, a.grpcBridgeHandler(a.governanceBridge)))
		authed.GET("/ops/readiness", a.grpcBridgeHandler(a.governanceBridge))
		authed.GET("/ops/dashboard", a.cacheJSONRoute("ops:dashboard:", opsDashboardTTL, a.grpcBridgeHandler(a.governanceBridge)))
		authed.GET("/ops/evidence", a.cacheJSONRoute("ops:evidence:", opsEvidenceTTL, a.grpcBridgeHandler(a.governanceBridge)))
		authed.POST("/ops/evidence", a.invalidateCacheOnSuccess(opsEvidenceWriteInvalidations, a.grpcBridgeHandler(a.governanceBridge)))
		authed.GET("/ops/incidents", a.cacheJSONRoute("ops:incidents:", opsIncidentsTTL, a.grpcBridgeHandler(a.governanceBridge)))
		authed.POST("/ops/incidents", a.invalidateCacheOnSuccess(opsIncidentWriteInvalidations, a.grpcBridgeHandler(a.governanceBridge)))
		authed.POST("/ops/incidents/:id/transition", a.invalidateCacheOnSuccess(opsIncidentWriteInvalidations, a.grpcBridgeHandler(a.governanceBridge)))
	} else if a.governanceServiceURL != "" {
		proxy := a.proxyTo(a.governanceServiceURL)
		authed.GET("/audits", a.cacheJSONRoute("audit:list:", auditListTTL, proxy))
		authed.GET("/analytics/admin-overview", a.cacheJSONRoute("analytics:", analyticsOverviewTTL, proxy))
		authed.GET("/ops/readiness", proxy)
		authed.GET("/ops/dashboard", a.cacheJSONRoute("ops:dashboard:", opsDashboardTTL, proxy))
		authed.GET("/ops/evidence", a.cacheJSONRoute("ops:evidence:", opsEvidenceTTL, proxy))
		authed.POST("/ops/evidence", a.invalidateCacheOnSuccess(opsEvidenceWriteInvalidations, proxy))
		authed.GET("/ops/incidents", a.cacheJSONRoute("ops:incidents:", opsIncidentsTTL, proxy))
		authed.POST("/ops/incidents", a.invalidateCacheOnSuccess(opsIncidentWriteInvalidations, proxy))
		authed.POST("/ops/incidents/:id/transition", a.invalidateCacheOnSuccess(opsIncidentWriteInvalidations, proxy))
	} else {
		if a.strictMicroservice {
			unavailable := serviceUnavailableHandler("governance-service upstream is not configured")
			authed.GET("/audits", unavailable)
			authed.GET("/analytics/admin-overview", unavailable)
			authed.GET("/ops/readiness", unavailable)
			authed.GET("/ops/dashboard", unavailable)
			authed.GET("/ops/evidence", unavailable)
			authed.POST("/ops/evidence", unavailable)
			authed.GET("/ops/incidents", unavailable)
			authed.POST("/ops/incidents", unavailable)
			authed.POST("/ops/incidents/:id/transition", unavailable)
		} else {
			authed.GET("/audits", a.listAudits)
			authed.GET("/analytics/admin-overview", a.adminOverview)
			authed.GET("/ops/readiness", a.opsReadiness)
			authed.GET("/ops/dashboard", a.opsDashboard)
			authed.GET("/ops/evidence", a.listEvidence)
			authed.POST("/ops/evidence", a.createEvidence)
			authed.GET("/ops/incidents", a.listIncidents)
			authed.POST("/ops/incidents", a.createIncident)
			authed.POST("/ops/incidents/:id/transition", a.transitionIncident)
		}
	}
}

func serviceUnavailableHandler(message string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "Service unavailable",
			"hint":  message,
		})
	}
}
