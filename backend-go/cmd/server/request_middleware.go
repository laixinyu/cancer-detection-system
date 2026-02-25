package main

// 文件： cmd/server/request_middleware.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *app) rateLimitMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.inboundLimiter == nil {
			c.Next()
			return
		}
		key := strings.TrimSpace(c.ClientIP())
		if key == "" {
			key = "unknown"
		}
		if !a.inboundLimiter.Allow(key) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "Too many requests"})
			return
		}
		c.Next()
	}
}

func timeoutMiddleware(timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if timeout <= 0 {
			c.Next()
			return
		}

		ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
		defer cancel()
		c.Request = c.Request.WithContext(ctx)
		c.Next()

		if errors.Is(ctx.Err(), context.DeadlineExceeded) && !c.Writer.Written() {
			c.AbortWithStatusJSON(http.StatusGatewayTimeout, gin.H{"error": "Request timeout"})
		}
	}
}
