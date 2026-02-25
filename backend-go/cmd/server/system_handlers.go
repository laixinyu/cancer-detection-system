package main

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *app) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok", "service": "cancer-detection-backend-go", "timestamp": time.Now().UTC().Format(time.RFC3339)})
}

func (a *app) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 3*time.Second)
	defer cancel()

	dbReady := true
	dbErr := ""
	var n int64
	if err := a.orm.WithContext(ctx).Model(&ormUser{}).Limit(1).Count(&n).Error; err != nil {
		dbReady = false
		dbErr = err.Error()
	}

	aiStatus := "ok"
	aiReachable := true
	healthURL := a.aiService + "/health"
	resp, err := a.httpClient.Get(healthURL)
	if err != nil {
		aiReachable = false
		aiStatus = err.Error()
	} else {
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			aiReachable = false
			aiStatus = fmt.Sprintf("HTTP_%d", resp.StatusCode)
		}
	}

	ready := dbReady && aiReachable
	status := "ready"
	code := http.StatusOK
	if !ready {
		status = "not_ready"
		code = http.StatusServiceUnavailable
		a.logger.Warn("alert_readiness_failed", "db_ready", dbReady, "ai_reachable", aiReachable, "db_error", dbErr, "ai_status", aiStatus, "alert_code", "READINESS_FAILED")
	}

	c.JSON(code, gin.H{
		"status":    status,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"database":  gin.H{"ready": dbReady, "error": dbErrOrNil(dbErr)},
		"ai":        gin.H{"reachable": aiReachable, "status": aiStatus},
	})
}
