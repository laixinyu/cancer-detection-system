package main

import "github.com/gin-gonic/gin"

func (a *app) registerRestRoutes(authed *gin.RouterGroup) {
	if a.detectionServiceURL != "" {
		proxy := a.proxyTo(a.detectionServiceURL)
		authed.GET("/detections", proxy)
		authed.GET("/detections/:id", proxy)
		authed.POST("/detections/:id/review", proxy)
	} else {
		authed.GET("/detections", a.listDetections)
		authed.GET("/detections/:id", a.getDetectionByID)
		authed.POST("/detections/:id/review", a.reviewDetection)
	}

	if a.reportServiceURL != "" {
		proxy := a.proxyTo(a.reportServiceURL)
		authed.GET("/reports", proxy)
		authed.GET("/reports/:id", proxy)
		authed.POST("/reports", proxy)
		authed.PATCH("/reports/:id", proxy)
	} else {
		authed.GET("/reports", a.listReports)
		authed.GET("/reports/:id", a.getReportByID)
		authed.POST("/reports", a.createReport)
		authed.PATCH("/reports/:id", a.updateReport)
	}

	if a.governanceServiceURL != "" {
		proxy := a.proxyTo(a.governanceServiceURL)
		authed.GET("/audits", proxy)
		authed.GET("/analytics/admin-overview", proxy)
		authed.GET("/ops/readiness", proxy)
		authed.GET("/ops/dashboard", proxy)
		authed.GET("/ops/evidence", proxy)
		authed.POST("/ops/evidence", proxy)
		authed.GET("/ops/incidents", proxy)
		authed.POST("/ops/incidents", proxy)
		authed.POST("/ops/incidents/:id/transition", proxy)
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
