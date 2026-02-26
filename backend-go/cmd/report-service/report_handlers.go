package main

import (
	"errors"
	"net/http"
	"strings"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type reportHandler struct {
	reportSvc *service.ReportService
}

func (h *reportHandler) listReports(c *gin.Context) {
	cl := mustClaims(c)
	status := strings.TrimSpace(c.Query("status"))
	patientID := strings.TrimSpace(c.Query("patientId"))
	limit := parseLimit(c.Query("limit"), 20)

	patientUserID := ""
	if cl.Role == "PATIENT" {
		patientUserID = cl.UserID
	}
	rows, next, err := h.reportSvc.List(c.Request.Context(), service.ListReportsInput{
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
		items = append(items, reportValueToMap(r))
	}
	var nextCursor any = nil
	if next != nil {
		nextCursor = *next
	}
	c.JSON(http.StatusOK, gin.H{"reports": items, "nextCursor": nextCursor})
}

func (h *reportHandler) getReport(c *gin.Context) {
	cl := mustClaims(c)
	row, err := h.reportSvc.GetByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Report not found"})
		return
	}
	if cl.Role == "PATIENT" && row.Patient.UserID != cl.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}
	c.JSON(http.StatusOK, reportToMap(row))
}

func (h *reportHandler) createReport(c *gin.Context) {
	cl := mustClaims(c)
	if cl.Role != "DOCTOR" && cl.Role != "ADMIN" {
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
	row, _, err := h.reportSvc.Upsert(c.Request.Context(), service.UpsertReportInput{
		DetectionID: req.DetectionID,
		PatientID:   req.PatientID,
		DoctorID:    cl.UserID,
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
	c.JSON(http.StatusOK, reportToMap(row))
}

func (h *reportHandler) updateReport(c *gin.Context) {
	cl := mustClaims(c)
	if cl.Role != "DOCTOR" && cl.Role != "ADMIN" {
		c.JSON(http.StatusForbidden, gin.H{"error": "Only doctors can update reports"})
		return
	}
	var req struct {
		Content *any   `json:"content"`
		Status  string `json:"status"`
		PDFPath string `json:"pdfPath"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	row, _, _, err := h.reportSvc.Update(c.Request.Context(), service.UpdateReportInput{
		ID:      c.Param("id"),
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
	c.JSON(http.StatusOK, reportToMap(row))
}

func reportValueToMap(r domain.Report) map[string]any {
	return map[string]any{
		"id":          r.ID,
		"detectionId": r.DetectionID,
		"patientId":   r.PatientID,
		"doctorId":    r.DoctorID,
		"content":     toIfaceMap(r.Content),
		"pdfPath":     strOrNil(r.PDFPath),
		"status":      r.Status,
		"createdAt":   r.CreatedAt,
		"updatedAt":   r.UpdatedAt,
	}
}

func reportToMap(r *domain.Report) map[string]any {
	if r == nil {
		return map[string]any{}
	}
	return reportValueToMap(*r)
}
