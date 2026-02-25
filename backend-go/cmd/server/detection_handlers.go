package main

// 文件： cmd/server/detection_handlers.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cancer-detection-backend/internal/service"

	"gorm.io/gorm"

	"github.com/gin-gonic/gin"
)

func (a *app) listDetections(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	priority := strings.TrimSpace(c.Query("priority"))
	orderByPriority := strings.EqualFold(c.DefaultQuery("orderByPriority", "true"), "true")
	limit := parseLimit(c, 20)
	cacheKey := "detection:list:v1:uid=" + claims.UserID + ":role=" + claims.Role + ":status=" + status + ":priority=" + priority + ":orderByPriority=" + c.DefaultQuery("orderByPriority", "true") + ":limit=" + strconv.Itoa(limit)
	var cached map[string]any
	if a.cacheGet(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	patientUserID := ""
	if claims.Role == "PATIENT" {
		patientUserID = claims.UserID
	}
	detections, next, err := a.detectionService.List(c.Request.Context(), service.ListDetectionsInput{
		Status:          status,
		Priority:        priority,
		OrderByPriority: orderByPriority,
		PatientUserID:   patientUserID,
		Limit:           limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list detections"})
		return
	}

	items := make([]map[string]any, 0, len(detections))
	for _, d := range detections {
		var reviewer any = nil
		if d.Reviewer != nil {
			reviewer = gin.H{"name": d.Reviewer.Name, "email": d.Reviewer.Email}
		}

		items = append(items, gin.H{
			"id":                d.ID,
			"imageId":           d.ImageID,
			"modelVersion":      d.ModelVersion,
			"cancerProbability": d.CancerProbability,
			"findings":          toIfaceMap(d.Findings),
			"heatmapPath":       strOrNil(d.HeatmapPath),
			"status":            d.Status,
			"reviewedBy":        strOrNil(d.ReviewedBy),
			"reviewNotes":       strOrNil(d.ReviewNotes),
			"createdAt":         d.CreatedAt,
			"updatedAt":         d.UpdatedAt,
			"image": gin.H{
				"id":           d.Image.ID,
				"patientId":    d.Image.PatientID,
				"filePath":     imageAccessPath(d.Image.ID),
				"fileType":     d.Image.FileType,
				"originalName": d.Image.OriginalName,
				"fileSize":     d.Image.FileSize,
				"status":       d.Image.Status,
				"uploadedBy":   d.Image.UploadedBy,
				"createdAt":    d.Image.CreatedAt,
				"updatedAt":    d.Image.UpdatedAt,
				"patient":      gin.H{"user": gin.H{"name": d.Image.Patient.User.Name, "email": d.Image.Patient.User.Email}},
			},
			"reviewer": reviewer,
		})
	}

	var nextCursor any = nil
	if next != nil {
		nextCursor = *next
	}
	response := gin.H{"detections": items, "nextCursor": nextCursor}
	a.cacheSet(c, cacheKey, response, 15*time.Second)
	c.JSON(http.StatusOK, response)
}

func (a *app) getDetectionByID(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	id := c.Param("id")

	d, err := a.detectionService.GetByID(c.Request.Context(), id)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Detection not found"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Detection not found"})
		return
	}
	if claims.Role == "PATIENT" && d.Image.UploadedBy != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	var reviewer any = nil
	if d.Reviewer != nil {
		reviewer = gin.H{"name": d.Reviewer.Name, "email": d.Reviewer.Email}
	}

	c.JSON(http.StatusOK, gin.H{
		"id":                d.ID,
		"imageId":           d.ImageID,
		"modelVersion":      d.ModelVersion,
		"cancerProbability": d.CancerProbability,
		"findings":          toIfaceMap(d.Findings),
		"heatmapPath":       strOrNil(d.HeatmapPath),
		"status":            d.Status,
		"reviewedBy":        strOrNil(d.ReviewedBy),
		"reviewNotes":       strOrNil(d.ReviewNotes),
		"createdAt":         d.CreatedAt,
		"updatedAt":         d.UpdatedAt,
		"image": gin.H{
			"id":           d.Image.ID,
			"patientId":    d.Image.PatientID,
			"filePath":     imageAccessPath(d.Image.ID),
			"fileType":     d.Image.FileType,
			"originalName": d.Image.OriginalName,
			"fileSize":     d.Image.FileSize,
			"status":       d.Image.Status,
			"uploadedBy":   d.Image.UploadedBy,
			"createdAt":    d.Image.CreatedAt,
			"updatedAt":    d.Image.UpdatedAt,
			"uploader":     gin.H{"id": d.Image.UploadedBy},
			"patient": gin.H{
				"id":          d.Image.Patient.ID,
				"dateOfBirth": d.Image.Patient.DateOfBirth,
				"gender":      d.Image.Patient.Gender,
				"user": gin.H{
					"id":        d.Image.Patient.User.ID,
					"email":     d.Image.Patient.User.Email,
					"name":      d.Image.Patient.User.Name,
					"role":      d.Image.Patient.User.Role,
					"phone":     strOrNil(d.Image.Patient.User.Phone),
					"createdAt": d.Image.Patient.User.CreatedAt,
					"updatedAt": d.Image.Patient.User.UpdatedAt,
				},
			},
		},
		"reviewer": reviewer,
	})
}

func (a *app) reviewDetection(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if claims.Role != "DOCTOR" && claims.Role != "ADMIN" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors can review detections"})
		return
	}

	id := c.Param("id")
	var req struct {
		Status      string `json:"status"`
		ReviewNotes string `json:"reviewNotes"`
		Findings    any    `json:"findings"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	req.Status = strings.TrimSpace(req.Status)
	if req.Status != "REVIEWED" && req.Status != "CONFIRMED" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		return
	}

	det, oldStatus, err := a.detectionService.Review(c.Request.Context(), service.ReviewDetectionInput{
		ID:          id,
		Status:      req.Status,
		ReviewerID:  claims.UserID,
		ReviewNotes: req.ReviewNotes,
		Findings:    req.Findings,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidDetectionStatus):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		case errors.Is(err, service.ErrInvalidDetectionTransition):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid detection status transition"})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Detection not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to review detection"})
		}
		return
	}

	a.writeAudit(c, claims, "DETECTION_REVIEW", "detection", det.ID, "SUCCESS", map[string]any{"fromStatus": oldStatus, "toStatus": det.Status})
	c.JSON(http.StatusOK, gin.H{
		"id":                det.ID,
		"imageId":           det.ImageID,
		"modelVersion":      det.ModelVersion,
		"cancerProbability": det.CancerProbability,
		"findings":          toIfaceMap(det.Findings),
		"heatmapPath":       strOrNil(det.HeatmapPath),
		"status":            det.Status,
		"reviewedBy":        strOrNil(det.ReviewedBy),
		"reviewNotes":       strOrNil(det.ReviewNotes),
		"createdAt":         det.CreatedAt,
		"updatedAt":         det.UpdatedAt,
	})
	a.cacheInvalidatePrefixes(c, "detection:list:", "report:list:", "audit:list:", "analytics:", "ops:dashboard:")
}
