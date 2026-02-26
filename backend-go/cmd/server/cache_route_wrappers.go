package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

type cachedRouteResponse struct {
	Status int `json:"status"`
	Body   any `json:"body"`
}

type bodyCaptureWriter struct {
	gin.ResponseWriter
	body bytes.Buffer
}

func (w *bodyCaptureWriter) Write(data []byte) (int, error) {
	_, _ = w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *bodyCaptureWriter) WriteString(s string) (int, error) {
	_, _ = w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

func (a *app) cacheJSONRoute(prefix string, ttl time.Duration, next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request.Method != http.MethodGet || a.cache == nil {
			next(c)
			return
		}

		key := a.routeCacheKey(prefix, c)
		var cached cachedRouteResponse
		if a.cacheGet(c, key, &cached) && cached.Status > 0 {
			c.JSON(cached.Status, cached.Body)
			return
		}

		capture := &bodyCaptureWriter{ResponseWriter: c.Writer}
		c.Writer = capture
		next(c)

		status := c.Writer.Status()
		if status < http.StatusOK || status >= http.StatusMultipleChoices {
			return
		}
		if capture.body.Len() == 0 || !json.Valid(capture.body.Bytes()) {
			return
		}

		var body any
		if err := json.Unmarshal(capture.body.Bytes(), &body); err != nil {
			return
		}
		a.cacheSet(c, key, cachedRouteResponse{
			Status: status,
			Body:   body,
		}, ttl)
	}
}

func (a *app) invalidateCacheOnSuccess(prefixes []string, next gin.HandlerFunc) gin.HandlerFunc {
	return func(c *gin.Context) {
		next(c)
		status := c.Writer.Status()
		if status >= http.StatusOK && status < http.StatusBadRequest {
			a.cacheInvalidatePrefixes(c, prefixes...)
			a.enqueueOutboxEvent(c)
		}
	}
}

func (a *app) routeCacheKey(prefix string, c *gin.Context) string {
	claims := getClaims(c)
	scope := "uid=anonymous:role=ANONYMOUS"
	if claims != nil {
		scope = "uid=" + strings.TrimSpace(claims.UserID) + ":role=" + strings.TrimSpace(claims.Role)
	}
	pattern := c.FullPath()
	if pattern == "" {
		pattern = c.Request.URL.Path
	}
	return fmt.Sprintf("%sroute:v1:%s:path=%s:query=%s", prefix, scope, pattern, c.Request.URL.RawQuery)
}
