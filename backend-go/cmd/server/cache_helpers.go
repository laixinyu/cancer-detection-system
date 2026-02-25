package main

import (
	"encoding/json"
	"time"

	"github.com/gin-gonic/gin"
)

func (a *app) cacheGet(c *gin.Context, key string, out any) bool {
	if a.cache == nil {
		return false
	}
	raw, hit, err := a.cache.Get(c.Request.Context(), key)
	if err != nil || !hit {
		return false
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return false
	}
	return true
}

func (a *app) cacheSet(c *gin.Context, key string, value any, ttl time.Duration) {
	if a.cache == nil {
		return
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return
	}
	_ = a.cache.Set(c.Request.Context(), key, raw, ttl)
}

func (a *app) cacheInvalidatePrefixes(c *gin.Context, prefixes ...string) {
	if a.cache == nil {
		return
	}
	for _, p := range prefixes {
		_ = a.cache.DeleteByPrefix(c.Request.Context(), p)
	}
}
