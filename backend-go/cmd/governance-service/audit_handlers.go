package main

import (
	"net/http"
	"strings"

	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
)

func (h *governanceHandler) listAudits(c *gin.Context) {
	cl := mustClaims(c)
	if cl.Role != "ADMIN" && cl.Role != "DOCTOR" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors and admins can view audit logs"})
		return
	}
	out, err := h.app.auditSvc.ListAudits(c.Request.Context(), service.ListAuditsInput{
		Action:     strings.TrimSpace(c.Query("action")),
		EntityType: strings.TrimSpace(c.Query("entityType")),
		EntityID:   strings.TrimSpace(c.Query("entityId")),
		Result:     strings.TrimSpace(c.Query("result")),
		Limit:      parseLimit(c.Query("limit"), 20),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list audit logs"})
		return
	}
	items := make([]map[string]any, 0, len(out.Logs))
	for _, row := range out.Logs {
		var actorUser any = nil
		if row.ActorUser != nil {
			actorUser = gin.H{"id": row.ActorUser.ID, "name": row.ActorUser.Name, "email": row.ActorUser.Email, "role": row.ActorUser.Role}
		}
		items = append(items, gin.H{
			"id":          row.ID,
			"actorUserId": strOrNil(row.ActorUserID),
			"actorRole":   strOrNil(row.ActorRole),
			"action":      row.Action,
			"entityType":  row.EntityType,
			"entityId":    row.EntityID,
			"result":      row.Result,
			"metadata":    row.Metadata,
			"createdAt":   row.CreatedAt,
			"actorUser":   actorUser,
		})
	}
	var nextCursor any = nil
	if out.NextCursor != nil {
		nextCursor = *out.NextCursor
	}
	c.JSON(http.StatusOK, gin.H{"logs": items, "nextCursor": nextCursor})
}
