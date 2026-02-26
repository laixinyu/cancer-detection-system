package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const idempotencyHeader = "Idempotency-Key"

func (a *app) idempotencyMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !a.idempotencyEnabled || a.idempotencyRepo == nil || !isIdempotentProtectedMethod(c.Request.Method) {
			c.Next()
			return
		}

		key := strings.TrimSpace(c.GetHeader(idempotencyHeader))
		if key == "" {
			if a.idempotencyRequired {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Missing Idempotency-Key header"})
				return
			}
			c.Next()
			return
		}

		rawBody := []byte{}
		if c.Request.Body != nil {
			payload, err := io.ReadAll(c.Request.Body)
			if err != nil {
				c.AbortWithStatusJSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
				return
			}
			rawBody = payload
			c.Request.Body = io.NopCloser(bytes.NewReader(payload))
		}
		scope := a.routeCacheKey("idem:", c)
		requestHash := hashRequest(c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery, rawBody)

		if existing, err := a.idempotencyRepo.Get(c.Request.Context(), scope, key); err == nil {
			if existing.RequestHash != requestHash {
				c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "Idempotency-Key reused with different payload"})
				return
			}
			if existing.ResponseStatus > 0 {
				if len(existing.ResponseBody) == 0 {
					c.Status(existing.ResponseStatus)
				} else {
					c.Data(existing.ResponseStatus, "application/json; charset=utf-8", existing.ResponseBody)
				}
				c.Abort()
				return
			}
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "Request with same Idempotency-Key is in progress"})
			return
		} else if err != nil && err != gorm.ErrRecordNotFound {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Idempotency validation failed"})
			return
		}

		rec, created, err := a.idempotencyRepo.CreatePending(c.Request.Context(), scope, key, requestHash, time.Now().UTC().Add(a.idempotencyTTL))
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Idempotency reservation failed"})
			return
		}
		if !created {
			c.AbortWithStatusJSON(http.StatusConflict, gin.H{"error": "Request with same Idempotency-Key already exists"})
			return
		}

		c.Set("idempotency_key", key)
		capture := &bodyCaptureWriter{ResponseWriter: c.Writer}
		c.Writer = capture
		c.Next()

		status := c.Writer.Status()
		if status >= http.StatusInternalServerError {
			_ = a.idempotencyRepo.Delete(c.Request.Context(), rec.ID)
			return
		}
		_ = a.idempotencyRepo.SaveResponse(c.Request.Context(), rec.ID, status, capture.body.Bytes())
	}
}

func isIdempotentProtectedMethod(method string) bool {
	switch strings.ToUpper(strings.TrimSpace(method)) {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func hashRequest(method, path, rawQuery string, body []byte) string {
	h := sha256.New()
	_, _ = h.Write([]byte(strings.ToUpper(method)))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(path))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write([]byte(rawQuery))
	_, _ = h.Write([]byte("|"))
	_, _ = h.Write(body)
	return hex.EncodeToString(h.Sum(nil))
}

func idempotencyKeyFromContext(c *gin.Context) *string {
	raw, ok := c.Get("idempotency_key")
	if !ok {
		return nil
	}
	key, ok := raw.(string)
	if !ok || strings.TrimSpace(key) == "" {
		return nil
	}
	out := strings.TrimSpace(key)
	return &out
}
