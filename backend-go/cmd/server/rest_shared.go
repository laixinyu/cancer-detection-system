package main

// 文件： cmd/server/rest_shared.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
)

func parseLimit(c *gin.Context, fallback int) int {
	v := strings.TrimSpace(c.Query("limit"))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return fallback
	}
	if n > 100 {
		return 100
	}
	return n
}

func strOrNil(v *string) any {
	if v == nil {
		return nil
	}
	return *v
}

func strOrEmpty(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}

func toIfaceMap(raw []byte) any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var out any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func imageAccessPath(imageID string) string {
	return "/api/images/" + imageID + "/file"
}

func (a *app) assertAdmin(c *gin.Context, claims *authClaims) bool {
	if claims == nil || claims.Role != "ADMIN" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only admins can access this endpoint"})
		return false
	}
	return true
}

func (a *app) writeAudit(c *gin.Context, claims *authClaims, action, entityType, entityID, result string, metadata map[string]any) {
	var actorUserID *string
	var actorRole *string
	if claims != nil {
		actorUserID = &claims.UserID
		actorRole = &claims.Role
	}
	_ = a.auditService.WriteAudit(c.Request.Context(), service.WriteAuditInput{
		ActorUserID: actorUserID,
		ActorRole:   actorRole,
		Action:      action,
		EntityType:  entityType,
		EntityID:    entityID,
		Result:      result,
		Metadata:    metadata,
	})
	a.cacheInvalidatePrefixes(c, "audit:list:", "analytics:")
}
