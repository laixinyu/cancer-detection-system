package main

// 文件： cmd/server/rest_routes.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import "github.com/gin-gonic/gin"

func (a *app) registerRestRoutes(authed *gin.RouterGroup) {
	if a.detectionBridge != nil {
		authed.GET("/detections", a.grpcBridgeHandler(a.detectionBridge))
		authed.GET("/detections/:id", a.grpcBridgeHandler(a.detectionBridge))
		authed.POST("/detections/:id/review", a.grpcBridgeHandler(a.detectionBridge))
	} else if a.detectionServiceURL != "" {
		proxy := a.proxyTo(a.detectionServiceURL)
		authed.GET("/detections", proxy)
		authed.GET("/detections/:id", proxy)
		authed.POST("/detections/:id/review", proxy)
	} else {
		authed.GET("/detections", a.listDetections)
		authed.GET("/detections/:id", a.getDetectionByID)
		authed.POST("/detections/:id/review", a.reviewDetection)
	}

	if a.reportBridge != nil {
		authed.GET("/reports", a.grpcBridgeHandler(a.reportBridge))
		authed.GET("/reports/:id", a.grpcBridgeHandler(a.reportBridge))
		authed.POST("/reports", a.grpcBridgeHandler(a.reportBridge))
		authed.PATCH("/reports/:id", a.grpcBridgeHandler(a.reportBridge))
	} else if a.reportServiceURL != "" {
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

	if a.governanceBridge != nil {
		authed.GET("/audits", a.grpcBridgeHandler(a.governanceBridge))
		authed.GET("/analytics/admin-overview", a.grpcBridgeHandler(a.governanceBridge))
		authed.GET("/ops/readiness", a.grpcBridgeHandler(a.governanceBridge))
		authed.GET("/ops/dashboard", a.grpcBridgeHandler(a.governanceBridge))
		authed.GET("/ops/evidence", a.grpcBridgeHandler(a.governanceBridge))
		authed.POST("/ops/evidence", a.grpcBridgeHandler(a.governanceBridge))
		authed.GET("/ops/incidents", a.grpcBridgeHandler(a.governanceBridge))
		authed.POST("/ops/incidents", a.grpcBridgeHandler(a.governanceBridge))
		authed.POST("/ops/incidents/:id/transition", a.grpcBridgeHandler(a.governanceBridge))
	} else if a.governanceServiceURL != "" {
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
