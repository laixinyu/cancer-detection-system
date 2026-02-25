package main

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
)

func (a *app) listAudits(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if claims.Role != "ADMIN" && claims.Role != "DOCTOR" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors and admins can view audit logs"})
		return
	}

	action := strings.TrimSpace(c.Query("action"))
	entityType := strings.TrimSpace(c.Query("entityType"))
	entityID := strings.TrimSpace(c.Query("entityId"))
	result := strings.TrimSpace(c.Query("result"))
	limit := parseLimit(c, 20)
	cacheKey := "audit:list:v1:action=" + action + ":entityType=" + entityType + ":entityId=" + entityID + ":result=" + result + ":limit=" + strconv.Itoa(limit)
	var cached map[string]any
	if a.cacheGet(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	out, err := a.auditService.ListAudits(c.Request.Context(), service.ListAuditsInput{
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		Result:     result,
		Limit:      limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list audit logs"})
		return
	}

	items := make([]map[string]any, 0, len(out.Logs))
	for _, logRow := range out.Logs {
		var actorUser any = nil
		if logRow.ActorUser != nil {
			actorUser = gin.H{
				"id":    logRow.ActorUser.ID,
				"name":  logRow.ActorUser.Name,
				"email": logRow.ActorUser.Email,
				"role":  logRow.ActorUser.Role,
			}
		}

		items = append(items, gin.H{
			"id":          logRow.ID,
			"actorUserId": strOrNil(logRow.ActorUserID),
			"actorRole":   strOrNil(logRow.ActorRole),
			"action":      logRow.Action,
			"entityType":  logRow.EntityType,
			"entityId":    logRow.EntityID,
			"result":      logRow.Result,
			"metadata":    logRow.Metadata,
			"createdAt":   logRow.CreatedAt,
			"actorUser":   actorUser,
		})
	}

	var nextCursor any = nil
	if out.NextCursor != nil {
		nextCursor = *out.NextCursor
	}

	response := gin.H{"logs": items, "nextCursor": nextCursor}
	a.cacheSet(c, cacheKey, response, 15*time.Second)
	c.JSON(http.StatusOK, response)
}
