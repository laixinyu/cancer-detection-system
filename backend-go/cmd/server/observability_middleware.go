package main

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const requestIDHeader = "X-Request-Id"

func requestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		reqID := strings.TrimSpace(c.GetHeader(requestIDHeader))
		if reqID == "" {
			reqID = uuid.NewString()
		}
		c.Set("request_id", reqID)
		c.Writer.Header().Set(requestIDHeader, reqID)
		c.Next()
	}
}

func (a *app) accessLogMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()
		route := c.FullPath()
		if route == "" {
			route = c.Request.URL.Path
		}
		reqID, _ := c.Get("request_id")
		uid := ""
		if claims := getClaims(c); claims != nil {
			uid = claims.UserID
		}

		a.metrics.ObserveHTTP(c.Request.Method, route, status, latency.Milliseconds())
		a.logger.Info("http_request",
			slog.String("request_id", toString(reqID)),
			slog.String("method", c.Request.Method),
			slog.String("route", route),
			slog.Int("status", status),
			slog.Int64("latency_ms", latency.Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
			slog.String("user_id", uid),
		)

		if status >= http.StatusInternalServerError || latency > a.slowRequestThreshold {
			a.logger.Warn("alert_http_request",
				slog.String("request_id", toString(reqID)),
				slog.String("method", c.Request.Method),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Int64("latency_ms", latency.Milliseconds()),
				slog.String("alert_code", alertCode(status, latency, a.slowRequestThreshold)),
			)
		}
	}
}

func (a *app) metricsHandler(c *gin.Context) {
	c.Data(http.StatusOK, "text/plain; version=0.0.4", []byte(a.metrics.PrometheusText()))
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func alertCode(status int, latency, threshold time.Duration) string {
	if status >= http.StatusInternalServerError {
		return "HIGH_5XX"
	}
	if latency > threshold {
		return "SLOW_REQUEST"
	}
	return "UNKNOWN"
}
