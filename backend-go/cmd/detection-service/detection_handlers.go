package main

import (
	"errors"
	"net/http"
	"strings"

	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type detectionHandler struct {
	detectionSvc *service.DetectionService
}

func (h *detectionHandler) listDetections(c *gin.Context) {
	cl := mustClaims(c)
	status := strings.TrimSpace(c.Query("status"))
	priority := strings.TrimSpace(c.Query("priority"))
	orderByPriority := strings.EqualFold(c.DefaultQuery("orderByPriority", "true"), "true")
	limit := parseLimit(c.Query("limit"), 20)

	patientUserID := ""
	if cl.Role == "PATIENT" {
		patientUserID = cl.UserID
	}
	rows, next, err := h.detectionSvc.List(c.Request.Context(), service.ListDetectionsInput{
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

	items := make([]map[string]any, 0, len(rows))
	for _, d := range rows {
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
	c.JSON(http.StatusOK, gin.H{"detections": items, "nextCursor": nextCursor})
}

func (h *detectionHandler) getDetection(c *gin.Context) {
	cl := mustClaims(c)
	id := c.Param("id")
	d, err := h.detectionSvc.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Detection not found"})
		return
	}
	if cl.Role == "PATIENT" && d.Image.UploadedBy != cl.UserID {
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
		"reviewer":          reviewer,
	})
}

func (h *detectionHandler) reviewDetection(c *gin.Context) {
	cl := mustClaims(c)
	if cl.Role != "DOCTOR" && cl.Role != "ADMIN" {
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
	row, _, err := h.detectionSvc.Review(c.Request.Context(), service.ReviewDetectionInput{
		ID:          id,
		Status:      req.Status,
		ReviewerID:  cl.UserID,
		ReviewNotes: req.ReviewNotes,
		Findings:    req.Findings,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidDetectionStatus), errors.Is(err, service.ErrInvalidDetectionTransition):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Detection not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to review detection"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":                row.ID,
		"imageId":           row.ImageID,
		"modelVersion":      row.ModelVersion,
		"cancerProbability": row.CancerProbability,
		"findings":          toIfaceMap(row.Findings),
		"heatmapPath":       strOrNil(row.HeatmapPath),
		"status":            row.Status,
		"reviewedBy":        strOrNil(row.ReviewedBy),
		"reviewNotes":       strOrNil(row.ReviewNotes),
		"createdAt":         row.CreatedAt,
		"updatedAt":         row.UpdatedAt,
	})
}
