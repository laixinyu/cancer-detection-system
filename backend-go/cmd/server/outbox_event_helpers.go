package main

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *app) enqueueOutboxEvent(c *gin.Context) {
	if a.outboxRepo == nil {
		return
	}

	route := c.FullPath()
	if route == "" {
		route = c.Request.URL.Path
	}
	claims := getClaims(c)
	userID := ""
	role := ""
	if claims != nil {
		userID = claims.UserID
		role = claims.Role
	}
	payload, err := json.Marshal(gin.H{
		"method":      c.Request.Method,
		"route":       route,
		"path":        c.Request.URL.Path,
		"query":       c.Request.URL.RawQuery,
		"userId":      userID,
		"userRole":    role,
		"requestId":   toStringMust(c.Get("request_id")),
		"status":      c.Writer.Status(),
		"occurredAt":  time.Now().UTC().Format(time.RFC3339Nano),
		"serviceMode": microserviceModeLabel(a.strictMicroservice),
	})
	if err != nil {
		return
	}

	aggregateType := normalizeAggregateType(route)
	aggregateID := strings.TrimSpace(c.Param("id"))
	if aggregateID == "" {
		aggregateID = "n/a"
	}
	eventType := strings.ToLower(strings.TrimSpace(c.Request.Method)) + "." + aggregateType + ".v1"
	if err := a.outboxRepo.Enqueue(c.Request.Context(), eventType, aggregateType, aggregateID, payload, idempotencyKeyFromContext(c)); err != nil {
		a.logger.Warn("outbox_enqueue_failed", "route", route, "error", err.Error())
	}
}

func normalizeAggregateType(route string) string {
	switch {
	case strings.Contains(route, "/detections"):
		return "detection"
	case strings.Contains(route, "/reports"):
		return "report"
	case strings.Contains(route, "/ops/incidents"):
		return "ops_incident"
	case strings.Contains(route, "/ops/evidence"):
		return "ops_evidence"
	case strings.Contains(route, "/images"):
		return "image"
	default:
		return "gateway_write"
	}
}

func microserviceModeLabel(strict bool) string {
	if strict {
		return "strict"
	}
	return "compat"
}

func toStringMust(value any, ok bool) string {
	if !ok {
		return ""
	}
	out, _ := value.(string)
	return out
}
