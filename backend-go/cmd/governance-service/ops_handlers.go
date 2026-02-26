package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/repository"
	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (h *governanceHandler) readiness(c *gin.Context) {
	if !assertAdmin(c, mustClaims(c)) {
		return
	}
	dbReady := true
	dbErr := ""
	if err := h.app.opsSvc.DBReady(c.Request.Context()); err != nil {
		dbReady = false
		dbErr = err.Error()
	}
	start := time.Now()
	aiReachable := true
	aiStatus := "ok"
	var aiHealth any = nil
	resp, err := h.app.httpClient.Get(h.app.aiServiceURL + "/health")
	if err != nil {
		aiReachable = false
		aiStatus = err.Error()
	} else {
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			aiReachable = false
			aiStatus = fmt.Sprintf("HTTP_%d", resp.StatusCode)
		} else {
			_ = json.NewDecoder(resp.Body).Decode(&aiHealth)
		}
	}
	latestEvidence, evidenceGate, err := loadLatestEvidenceAndGate(c.Request.Context(), h.app.evidenceStore)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load readiness evidence"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"timestamp":      time.Now().UTC().Format(time.RFC3339),
		"overallReady":   dbReady && aiReachable,
		"db":             gin.H{"ready": dbReady, "error": dbErrOrNil(dbErr)},
		"ai":             gin.H{"aiReachable": aiReachable, "aiStatus": aiStatus, "aiHealth": aiHealth, "latencyMs": time.Since(start).Milliseconds()},
		"evidenceGate":   evidenceGate,
		"latestEvidence": latestEvidence,
	})
}

func (h *governanceHandler) dashboard(c *gin.Context) {
	if !assertAdmin(c, mustClaims(c)) {
		return
	}
	latestEvidence, evidenceGate, err := loadLatestEvidenceAndGate(c.Request.Context(), h.app.evidenceStore)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load ops dashboard"})
		return
	}
	openIncidents, p0p1Incidents, err := h.app.opsSvc.DashboardCounts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load ops dashboard"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"latestEvidence": latestEvidence,
		"evidenceGate":   evidenceGate,
		"openIncidents":  openIncidents,
		"p0p1Incidents":  p0p1Incidents,
	})
}

func (h *governanceHandler) listEvidence(c *gin.Context) {
	if !assertAdmin(c, mustClaims(c)) {
		return
	}
	rows, err := h.app.evidenceStore.ListEvidence(c.Request.Context(), parseLimit(c.Query("limit"), 20))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list evidence"})
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		items = append(items, evidenceRecordToMap(row))
	}
	c.JSON(http.StatusOK, items)
}

func (h *governanceHandler) createEvidence(c *gin.Context) {
	cl := mustClaims(c)
	if !assertAdmin(c, cl) {
		return
	}
	var req struct {
		RunName                  string   `json:"runName"`
		ModelVersion             string   `json:"modelVersion"`
		DatasetName              string   `json:"datasetName"`
		DatasetVersion           string   `json:"datasetVersion"`
		SampleCount              int      `json:"sampleCount"`
		PositiveCount            int      `json:"positiveCount"`
		SiteCount                int      `json:"siteCount"`
		Auroc                    float64  `json:"auroc"`
		Sensitivity              float64  `json:"sensitivity"`
		Specificity              float64  `json:"specificity"`
		PPV                      *float64 `json:"ppv"`
		NPV                      *float64 `json:"npv"`
		ECE                      *float64 `json:"ece"`
		Brier                    *float64 `json:"brier"`
		CalibrationTemperature   *float64 `json:"calibrationTemperature"`
		ThresholdHighSensitivity *float64 `json:"thresholdHighSensitivity"`
		ThresholdHighSpecificity *float64 `json:"thresholdHighSpecificity"`
		StageRecommendation      string   `json:"stageRecommendation"`
		RegulatoryStatus         string   `json:"regulatoryStatus"`
		QAApprovedBy             string   `json:"qaApprovedBy"`
		MedicalApprovedBy        string   `json:"medicalApprovedBy"`
		ReportPath               string   `json:"reportPath"`
		Notes                    string   `json:"notes"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if strings.TrimSpace(req.RunName) == "" || strings.TrimSpace(req.ModelVersion) == "" || strings.TrimSpace(req.DatasetName) == "" || req.SampleCount <= 0 || req.SiteCount <= 0 || req.PositiveCount < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid evidence payload"})
		return
	}
	if req.StageRecommendation == "" {
		req.StageRecommendation = "RESEARCH_ONLY"
	}
	if req.RegulatoryStatus == "" {
		req.RegulatoryStatus = "NOT_SUBMITTED"
	}
	row := &domain.ClinicalEvidenceRun{
		RunName:                  strings.TrimSpace(req.RunName),
		ModelVersion:             strings.TrimSpace(req.ModelVersion),
		DatasetName:              strings.TrimSpace(req.DatasetName),
		DatasetVersion:           toStrPtr(req.DatasetVersion),
		SampleCount:              req.SampleCount,
		PositiveCount:            req.PositiveCount,
		SiteCount:                req.SiteCount,
		Auroc:                    req.Auroc,
		Sensitivity:              req.Sensitivity,
		Specificity:              req.Specificity,
		PPV:                      req.PPV,
		NPV:                      req.NPV,
		ECE:                      req.ECE,
		Brier:                    req.Brier,
		CalibrationTemperature:   req.CalibrationTemperature,
		ThresholdHighSensitivity: req.ThresholdHighSensitivity,
		ThresholdHighSpecificity: req.ThresholdHighSpecificity,
		StageRecommendation:      req.StageRecommendation,
		RegulatoryStatus:         req.RegulatoryStatus,
		QAApprovedBy:             toStrPtr(req.QAApprovedBy),
		MedicalApprovedBy:        toStrPtr(req.MedicalApprovedBy),
		ReportPath:               toStrPtr(req.ReportPath),
		Notes:                    toStrPtr(req.Notes),
		CreatedByUserID:          &cl.UserID,
	}
	if err := h.app.opsSvc.CreateEvidence(c.Request.Context(), row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create evidence"})
		return
	}
	c.JSON(http.StatusOK, evidenceRecordToMap(repository.EvidenceRecord{
		ID:                       row.ID,
		RunName:                  row.RunName,
		ModelVersion:             row.ModelVersion,
		DatasetName:              row.DatasetName,
		DatasetVersion:           row.DatasetVersion,
		SampleCount:              row.SampleCount,
		PositiveCount:            row.PositiveCount,
		SiteCount:                row.SiteCount,
		Auroc:                    row.Auroc,
		Sensitivity:              row.Sensitivity,
		Specificity:              row.Specificity,
		PPV:                      row.PPV,
		NPV:                      row.NPV,
		ECE:                      row.ECE,
		Brier:                    row.Brier,
		CalibrationTemperature:   row.CalibrationTemperature,
		ThresholdHighSensitivity: row.ThresholdHighSensitivity,
		ThresholdHighSpecificity: row.ThresholdHighSpecificity,
		RegulatoryStatus:         row.RegulatoryStatus,
		StageRecommendation:      row.StageRecommendation,
		QAApprovedBy:             row.QAApprovedBy,
		MedicalApprovedBy:        row.MedicalApprovedBy,
		ReportPath:               row.ReportPath,
		Notes:                    row.Notes,
		CreatedByUserID:          row.CreatedByUserID,
		CreatedAt:                row.CreatedAt,
		UpdatedAt:                row.UpdatedAt,
	}))
}

func (h *governanceHandler) listIncidents(c *gin.Context) {
	if !assertAdmin(c, mustClaims(c)) {
		return
	}
	rows, err := h.app.opsSvc.ListIncidents(c.Request.Context(), strings.TrimSpace(c.Query("status")), parseLimit(c.Query("limit"), 30))
	if err != nil {
		if errors.Is(err, service.ErrInvalidIncidentStatus) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list incidents"})
		return
	}
	items := make([]map[string]any, 0, len(rows))
	for _, i := range rows {
		var owner any = nil
		if i.Owner != nil {
			owner = gin.H{"id": i.Owner.ID, "name": i.Owner.Name, "email": i.Owner.Email}
		}
		items = append(items, gin.H{
			"id":             i.ID,
			"source":         i.Source,
			"severity":       i.Severity,
			"status":         i.Status,
			"title":          i.Title,
			"detail":         strOrNil(i.Detail),
			"ownerUserId":    strOrNil(i.OwnerUserID),
			"openedAt":       i.OpenedAt,
			"acknowledgedAt": i.AcknowledgedAt,
			"resolvedAt":     i.ResolvedAt,
			"createdAt":      i.CreatedAt,
			"updatedAt":      i.UpdatedAt,
			"owner":          owner,
		})
	}
	c.JSON(http.StatusOK, items)
}

func (h *governanceHandler) createIncident(c *gin.Context) {
	if !assertAdmin(c, mustClaims(c)) {
		return
	}
	var req struct {
		Source      string `json:"source"`
		Severity    string `json:"severity"`
		Title       string `json:"title"`
		Detail      string `json:"detail"`
		OwnerUserID string `json:"ownerUserId"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	row, err := h.app.opsSvc.CreateIncident(c.Request.Context(), service.CreateIncidentInput{
		Source:      req.Source,
		Severity:    req.Severity,
		Title:       req.Title,
		Detail:      req.Detail,
		OwnerUserID: req.OwnerUserID,
	})
	if err != nil {
		if errors.Is(err, service.ErrInvalidIncidentPayload) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid incident payload"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create incident"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":             row.ID,
		"source":         row.Source,
		"severity":       row.Severity,
		"status":         row.Status,
		"title":          row.Title,
		"detail":         strOrNil(row.Detail),
		"ownerUserId":    strOrNil(row.OwnerUserID),
		"openedAt":       row.OpenedAt,
		"acknowledgedAt": row.AcknowledgedAt,
		"resolvedAt":     row.ResolvedAt,
		"createdAt":      row.CreatedAt,
		"updatedAt":      row.UpdatedAt,
	})
}

func (h *governanceHandler) transitionIncident(c *gin.Context) {
	cl := mustClaims(c)
	if !assertAdmin(c, cl) {
		return
	}
	var req struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	row, _, err := h.app.opsSvc.TransitionIncident(c.Request.Context(), c.Param("id"), req.Action, cl.UserID)
	if err != nil {
		switch {
		case errors.Is(err, service.ErrInvalidIncidentTransition):
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid incident transition"})
		case errors.Is(err, gorm.ErrRecordNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "Incident not found"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to transition incident"})
		}
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":             row.ID,
		"source":         row.Source,
		"severity":       row.Severity,
		"status":         row.Status,
		"title":          row.Title,
		"detail":         strOrNil(row.Detail),
		"ownerUserId":    strOrNil(row.OwnerUserID),
		"openedAt":       row.OpenedAt,
		"acknowledgedAt": row.AcknowledgedAt,
		"resolvedAt":     row.ResolvedAt,
		"createdAt":      row.CreatedAt,
		"updatedAt":      row.UpdatedAt,
	})
}

func loadLatestEvidenceAndGate(ctx context.Context, store repository.EvidenceStore) (map[string]any, map[string]any, error) {
	rec, err := store.GetLatestEvidence(ctx)
	if err != nil {
		return nil, nil, err
	}
	if rec == nil {
		return nil, nil, nil
	}
	latest := evidenceRecordToMap(*rec)
	return latest, evaluateEvidenceGate(latest), nil
}

func evidenceRecordToMap(rec repository.EvidenceRecord) map[string]any {
	var createdBy any = nil
	if rec.CreatedBy != nil {
		createdBy = gin.H{"id": rec.CreatedBy.ID, "name": rec.CreatedBy.Name, "email": rec.CreatedBy.Email}
	}
	return map[string]any{
		"id":                       rec.ID,
		"runName":                  rec.RunName,
		"modelVersion":             rec.ModelVersion,
		"datasetName":              rec.DatasetName,
		"datasetVersion":           strOrNil(rec.DatasetVersion),
		"sampleCount":              rec.SampleCount,
		"positiveCount":            rec.PositiveCount,
		"siteCount":                rec.SiteCount,
		"auroc":                    rec.Auroc,
		"sensitivity":              rec.Sensitivity,
		"specificity":              rec.Specificity,
		"ppv":                      rec.PPV,
		"npv":                      rec.NPV,
		"ece":                      rec.ECE,
		"brier":                    rec.Brier,
		"calibrationTemperature":   rec.CalibrationTemperature,
		"thresholdHighSensitivity": rec.ThresholdHighSensitivity,
		"thresholdHighSpecificity": rec.ThresholdHighSpecificity,
		"regulatoryStatus":         rec.RegulatoryStatus,
		"stageRecommendation":      rec.StageRecommendation,
		"qaApprovedBy":             strOrNil(rec.QAApprovedBy),
		"medicalApprovedBy":        strOrNil(rec.MedicalApprovedBy),
		"reportPath":               strOrNil(rec.ReportPath),
		"notes":                    strOrNil(rec.Notes),
		"createdByUserId":          strOrNil(rec.CreatedByUserID),
		"createdAt":                rec.CreatedAt,
		"updatedAt":                rec.UpdatedAt,
		"createdBy":                createdBy,
	}
}

func evaluateEvidenceGate(evidence map[string]any) map[string]any {
	minSiteCount := parseIntEnv("AI_GOV_MIN_SITE_COUNT", 2)
	minAuroc := parseFloatEnv("AI_GOV_MIN_AUROC", 0.90)
	minSensitivity := parseFloatEnv("AI_GOV_MIN_SENSITIVITY", 0.90)
	minSpecificity := parseFloatEnv("AI_GOV_MIN_SPECIFICITY", 0.85)
	siteCount := intOrZero(anyToIntPtr(evidence["siteCount"]))
	auroc := floatOrZero(anyToFloatPtr(evidence["auroc"]))
	sensitivity := floatOrZero(anyToFloatPtr(evidence["sensitivity"]))
	specificity := floatOrZero(anyToFloatPtr(evidence["specificity"]))
	regulatoryStatus, _ := evidence["regulatoryStatus"].(string)
	pass := siteCount >= minSiteCount && auroc >= minAuroc && sensitivity >= minSensitivity && specificity >= minSpecificity && regulatoryStatus == "APPROVED"
	return map[string]any{
		"pass": pass,
		"criteria": map[string]any{
			"minSiteCount":   minSiteCount,
			"minAuroc":       minAuroc,
			"minSensitivity": minSensitivity,
			"minSpecificity": minSpecificity,
		},
	}
}
