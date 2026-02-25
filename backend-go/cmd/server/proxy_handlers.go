package main

// 文件： cmd/server/proxy_handlers.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"strings"

	"cancer-detection-backend/internal/resilience"

	"github.com/gin-gonic/gin"
)

func (a *app) proxyTo(baseURL string) gin.HandlerFunc {
	return func(c *gin.Context) {
		breaker := a.upstreamBreakers.For(baseURL)
		if err := breaker.Allow(); err != nil {
			if errors.Is(err, resilience.ErrCircuitOpen) {
				c.JSON(http.StatusServiceUnavailable, gin.H{"error": "Upstream temporarily unavailable"})
				return
			}
			c.JSON(http.StatusBadGateway, gin.H{"error": "Upstream unavailable"})
			return
		}
		targetURL := baseURL + c.Request.URL.Path
		if q := c.Request.URL.RawQuery; q != "" {
			targetURL += "?" + q
		}

		var body []byte
		if c.Request.Body != nil {
			payload, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			body = payload
			c.Request.Body = io.NopCloser(bytes.NewReader(payload))
		}

		req, err := http.NewRequestWithContext(c.Request.Context(), c.Request.Method, targetURL, bytes.NewReader(body))
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": "Failed to build upstream request"})
			return
		}

		copyHeaders(c.Request.Header, req.Header)
		resp, err := a.httpClient.Do(req)
		if err != nil {
			breaker.RecordFailure()
			c.JSON(http.StatusBadGateway, gin.H{"error": "Upstream service unavailable"})
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= http.StatusBadGateway {
			breaker.RecordFailure()
		} else {
			breaker.RecordSuccess()
		}

		copyResponseHeaders(resp.Header, c.Writer.Header())
		c.Status(resp.StatusCode)
		_, _ = io.Copy(c.Writer, resp.Body)
	}
}

func copyHeaders(src http.Header, dst http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

func copyResponseHeaders(src http.Header, dst http.Header) {
	for key, values := range src {
		if isHopByHopHeader(key) {
			continue
		}
		for _, v := range values {
			dst.Add(key, v)
		}
	}
}

func isHopByHopHeader(key string) bool {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "connection", "proxy-connection", "keep-alive", "transfer-encoding", "upgrade", "te", "trailers":
		return true
	default:
		return false
	}
}
