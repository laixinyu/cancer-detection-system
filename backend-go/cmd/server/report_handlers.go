package main

// File: cmd/server/report_handlers.go
// Purpose: Gateway handlers, middleware, and wiring for external HTTP APIs.

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (a *app) listReports(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	patientID := strings.TrimSpace(c.Query("patientId"))
	limit := parseLimit(c, 20)
	cacheKey := "report:list:v1:uid=" + claims.UserID + ":role=" + claims.Role + ":patientId=" + patientID + ":status=" + status + ":limit=" + strconv.Itoa(limit)
	var cached map[string]any
	if a.cacheGet(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	patientUserID := ""
	if claims.Role == "PATIENT" {
		patientUserID = claims.UserID
	}
	rows, next, err := a.reportService.List(c.Request.Context(), service.ListReportsInput{
		Status:        status,
		PatientID:     patientID,
		PatientUserID: patientUserID,
		Limit:         limit,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list reports"})
		return
	}

	items := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		items = append(items, gin.H{
			"id":          r.ID,
			"detectionId": r.DetectionID,
			"patientId":   r.PatientID,
			"doctorId":    r.DoctorID,
			"content":     toIfaceMap(r.Content),
			"pdfPath":     strOrNil(r.PDFPath),
			"status":      r.Status,
			"createdAt":   r.CreatedAt,
			"updatedAt":   r.UpdatedAt,
			"patient": gin.H{
				"id": r.Patient.ID,
				"user": gin.H{
					"name":  r.Patient.User.Name,
					"email": r.Patient.User.Email,
				},
			},
			"doctor": gin.H{
				"id":    r.Doctor.ID,
				"name":  r.Doctor.Name,
				"email": r.Doctor.Email,
			},
			"detection": gin.H{
				"id":                r.Detection.ID,
				"cancerProbability": r.Detection.CancerProbability,
				"image": gin.H{
					"id":           r.Detection.Image.ID,
					"originalName": r.Detection.Image.OriginalName,
					"filePath":     imageAccessPath(r.Detection.Image.ID),
				},
			},
		})
	}

	var nextCursor any = nil
	if next != nil {
		nextCursor = *next
	}

	response := gin.H{"reports": items, "nextCursor": nextCursor}
	a.cacheSet(c, cacheKey, response, 15*time.Second)
	c.JSON(http.StatusOK, response)
}

func (a *app) getReportByID(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	id := c.Param("id")

	r, err := a.reportService.GetByID(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
		return
	}

	if claims.Role == "PATIENT" && r.Patient.UserID != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":          r.ID,
		"detectionId": r.DetectionID,
		"patientId":   r.PatientID,
		"doctorId":    r.DoctorID,
		"content":     toIfaceMap(r.Content),
		"pdfPath":     strOrNil(r.PDFPath),
		"status":      r.Status,
		"createdAt":   r.CreatedAt,
		"updatedAt":   r.UpdatedAt,
		"patient": gin.H{
			"id":          r.Patient.ID,
			"dateOfBirth": r.Patient.DateOfBirth,
			"gender":      r.Patient.Gender,
			"user": gin.H{
				"id":        r.Patient.User.ID,
				"name":      r.Patient.User.Name,
				"email":     r.Patient.User.Email,
				"role":      r.Patient.User.Role,
				"phone":     strOrNil(r.Patient.User.Phone),
				"createdAt": r.Patient.User.CreatedAt,
				"updatedAt": r.Patient.User.UpdatedAt,
			},
		},
		"doctor": gin.H{
			"id":    r.Doctor.ID,
			"name":  r.Doctor.Name,
			"email": r.Doctor.Email,
		},
		"detection": gin.H{
			"id":                r.Detection.ID,
			"cancerProbability": r.Detection.CancerProbability,
			"status":            r.Detection.Status,
			"image": gin.H{
				"id":           r.Detection.Image.ID,
				"originalName": r.Detection.Image.OriginalName,
				"filePath":     imageAccessPath(r.Detection.Image.ID),
			},
		},
	})
}

func (a *app) createReport(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if claims.Role != "DOCTOR" && claims.Role != "ADMIN" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors can create reports"})
		return
	}

	var req struct {
		DetectionID string `json:"detectionId"`
		PatientID   string `json:"patientId"`
		Content     any    `json:"content"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	row, upserted, err := a.reportService.Upsert(c.Request.Context(), service.UpsertReportInput{
		DetectionID: req.DetectionID,
		PatientID:   req.PatientID,
		DoctorID:    claims.UserID,
		Content:     req.Content,
		Status:      req.Status,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidReportPayload), errors.Is(err, service.ErrInvalidReportStatus), errors.Is(err, service.ErrDetectionNotReviewed), errors.Is(err, service.ErrPatientMismatch):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Detection not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create report"})
		}
		return
	}

	if upserted {
		a.writeAudit(c, claims, "REPORT_UPSERT", "report", row.ID, "SUCCESS", map[string]any{"detectionId": row.DetectionID, "status": row.Status})
	} else {
		a.writeAudit(c, claims, "REPORT_CREATE", "report", row.ID, "SUCCESS", map[string]any{"status": row.Status, "detectionId": row.DetectionID})
	}
	c.JSON(http.StatusOK, gin.H{
		"id":          row.ID,
		"detectionId": row.DetectionID,
		"patientId":   row.PatientID,
		"doctorId":    row.DoctorID,
		"content":     toIfaceMap(row.Content),
		"pdfPath":     strOrNil(row.PDFPath),
		"status":      row.Status,
		"createdAt":   row.CreatedAt,
		"updatedAt":   row.UpdatedAt,
	})
	a.cacheInvalidatePrefixes(c, "report:list:", "analytics:", "audit:list:")
}

func (a *app) updateReport(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	if claims.Role != "DOCTOR" && claims.Role != "ADMIN" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors can update reports"})
		return
	}

	id := c.Param("id")
	var req struct {
		Content *any   `json:"content"`
		Status  string `json:"status"`
		PDFPath string `json:"pdfPath"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	row, oldStatus, updatedFields, err := a.reportService.Update(c.Request.Context(), service.UpdateReportInput{
		ID:      id,
		Content: req.Content,
		Status:  req.Status,
		PDFPath: req.PDFPath,
	})
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidReportStatus), errors.Is(err, service.ErrInvalidReportTransition), errors.Is(err, service.ErrNoReportFieldsToUpdate):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update report"})
		}
		return
	}

	a.writeAudit(c, claims, "REPORT_UPDATE", "report", row.ID, "SUCCESS", map[string]any{"fromStatus": oldStatus, "toStatus": row.Status, "updatedFields": updatedFields})
	c.JSON(http.StatusOK, gin.H{
		"id":          row.ID,
		"detectionId": row.DetectionID,
		"patientId":   row.PatientID,
		"doctorId":    row.DoctorID,
		"content":     toIfaceMap(row.Content),
		"pdfPath":     strOrNil(row.PDFPath),
		"status":      row.Status,
		"createdAt":   row.CreatedAt,
		"updatedAt":   row.UpdatedAt,
	})
	a.cacheInvalidatePrefixes(c, "report:list:", "analytics:", "audit:list:")
}
