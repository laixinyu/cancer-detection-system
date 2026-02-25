package main

// File: cmd/server/observability_middleware.go
// Purpose: Gateway handlers, middleware, and wiring for external HTTP APIs.

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
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
			slog.String("trace_id", traceIDFromContext(c.Request.Context())),
			slog.String("span_id", spanIDFromContext(c.Request.Context())),
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
				slog.String("trace_id", traceIDFromContext(c.Request.Context())),
				slog.String("method", c.Request.Method),
				slog.String("route", route),
				slog.Int("status", status),
				slog.Int64("latency_ms", latency.Milliseconds()),
				slog.String("alert_code", alertCode(status, latency, a.slowRequestThreshold)),
			)
		}
	}
}

func (a *app) traceMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.tracer == nil {
			c.Next()
			return
		}
		ctx := propagation.TraceContext{}.Extract(c.Request.Context(), propagation.HeaderCarrier(c.Request.Header))
		ctx, span := a.tracer.Start(ctx, c.Request.Method+" "+c.FullPath())
		span.SetAttributes(
			attribute.String("http.method", c.Request.Method),
			attribute.String("http.route", c.FullPath()),
			attribute.String("http.target", c.Request.URL.Path),
		)
		c.Request = c.Request.WithContext(ctx)
		defer func() {
			status := c.Writer.Status()
			span.SetAttributes(attribute.Int("http.status_code", status))
			if status >= http.StatusInternalServerError {
				span.SetStatus(codes.Error, http.StatusText(status))
			} else {
				span.SetStatus(codes.Ok, "")
			}
			span.End()
		}()
		c.Next()
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

func traceIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.TraceID().String()
}

func spanIDFromContext(ctx context.Context) string {
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return ""
	}
	return sc.SpanID().String()
}
