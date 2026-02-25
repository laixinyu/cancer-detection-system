package main

// File: cmd/server/ops_handlers.go
// Purpose: Gateway handlers, middleware, and wiring for external HTTP APIs.

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/repository"
	"cancer-detection-backend/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var (
	deploymentStages    = []string{"RESEARCH_ONLY", "PILOT_DECISION_SUPPORT", "CLINICAL_DECISION_SUPPORT"}
	regulatoryStatuses  = []string{"NOT_SUBMITTED", "SUBMITTED", "APPROVED", "REJECTED"}
	incidentSources     = []string{"AI_SERVICE", "APP", "DATABASE", "PIPELINE", "SECURITY", "OTHER"}
	incidentSeverities  = []string{"P0", "P1", "P2", "P3"}
	incidentStatuses    = []string{"OPEN", "ACKNOWLEDGED", "RESOLVED"}
	incidentTransitions = []string{"ACKNOWLEDGE", "RESOLVE", "REOPEN"}
)

func floatOrZero(v *float64) float64 {
	if v == nil {
		return 0
	}
	return *v
}

func intOrZero(v *int) int {
	if v == nil {
		return 0
	}
	return *v
}

func parseFloatEnv(name string, fallback float64) float64 {
	value := strings.TrimSpace(envOr(name, ""))
	if value == "" {
		return fallback
	}
	n, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return n
}

func parseIntEnv(name string, fallback int) int {
	value := strings.TrimSpace(envOr(name, ""))
	if value == "" {
		return fallback
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return n
}

func containsString(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
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

	pass := siteCount >= minSiteCount &&
		auroc >= minAuroc &&
		sensitivity >= minSensitivity &&
		specificity >= minSpecificity &&
		regulatoryStatus == "APPROVED"

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

func anyToFloatPtr(v any) *float64 {
	switch n := v.(type) {
	case float64:
		return &n
	case float32:
		x := float64(n)
		return &x
	case int:
		x := float64(n)
		return &x
	case int64:
		x := float64(n)
		return &x
	default:
		return nil
	}
}

func anyToIntPtr(v any) *int {
	switch n := v.(type) {
	case int:
		return &n
	case int64:
		x := int(n)
		return &x
	case float64:
		x := int(n)
		return &x
	default:
		return nil
	}
}

func evidenceRecordToMap(rec repository.EvidenceRecord) map[string]any {
	var createdBy any = nil
	if rec.CreatedBy != nil {
		createdBy = gin.H{
			"id":    rec.CreatedBy.ID,
			"name":  rec.CreatedBy.Name,
			"email": rec.CreatedBy.Email,
		}
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

func (a *app) loadLatestEvidenceAndGate(c *gin.Context) (map[string]any, map[string]any, error) {
	rec, err := a.evidenceStore.GetLatestEvidence(c.Request.Context())
	if err != nil {
		return nil, nil, err
	}
	if rec == nil {
		return nil, nil, nil
	}

	latest := evidenceRecordToMap(*rec)
	return latest, evaluateEvidenceGate(latest), nil
}

func (a *app) opsReadiness(c *gin.Context) {
	claims := getClaims(c)
	if !a.assertAdmin(c, claims) {
		return
	}

	dbReady := true
	dbErr := ""
	if err := a.opsService.DBReady(c.Request.Context()); err != nil {
		dbReady = false
		dbErr = err.Error()
	}

	start := time.Now()
	aiReachable := true
	aiStatus := "ok"
	var aiHealth any = nil

	healthURL := strings.TrimRight(a.aiService, "/") + "/health"
	resp, err := a.httpClient.Get(healthURL)
	if err != nil {
		aiReachable = false
		aiStatus = err.Error()
	} else {
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			aiReachable = false
			aiStatus = fmt.Sprintf("HTTP_%d", resp.StatusCode)
		} else {
			var body any
			if decodeErr := json.NewDecoder(resp.Body).Decode(&body); decodeErr == nil {
				aiHealth = body
			}
		}
	}

	latestEvidence, evidenceGate, err := a.loadLatestEvidenceAndGate(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load readiness evidence"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"timestamp":    time.Now().UTC().Format(time.RFC3339),
		"overallReady": dbReady && aiReachable,
		"db": gin.H{
			"ready": dbReady,
			"error": dbErrOrNil(dbErr),
		},
		"ai": gin.H{
			"aiReachable": aiReachable,
			"aiStatus":    aiStatus,
			"aiHealth":    aiHealth,
			"latencyMs":   time.Since(start).Milliseconds(),
		},
		"evidenceGate":   evidenceGate,
		"latestEvidence": latestEvidence,
	})
}

func (a *app) opsDashboard(c *gin.Context) {
	claims := getClaims(c)
	if !a.assertAdmin(c, claims) {
		return
	}
	cacheKey := "ops:dashboard:v1"
	var cached map[string]any
	if a.cacheGet(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	latestEvidence, evidenceGate, err := a.loadLatestEvidenceAndGate(c)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load ops dashboard"})
		return
	}

	openIncidents, p0p1Incidents, err := a.opsService.DashboardCounts(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load ops dashboard"})
		return
	}

	response := gin.H{
		"latestEvidence": latestEvidence,
		"evidenceGate":   evidenceGate,
		"openIncidents":  openIncidents,
		"p0p1Incidents":  p0p1Incidents,
	}
	a.cacheSet(c, cacheKey, response, 20*time.Second)
	c.JSON(http.StatusOK, response)
}

func (a *app) listEvidence(c *gin.Context) {
	claims := getClaims(c)
	if !a.assertAdmin(c, claims) {
		return
	}
	limit := parseLimit(c, 20)
	cacheKey := fmt.Sprintf("ops:evidence:list:v1:limit=%d", limit)
	var cached []map[string]any
	if a.cacheGet(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	records, err := a.evidenceStore.ListEvidence(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list evidence"})
		return
	}

	items := make([]map[string]any, 0, len(records))
	for _, rec := range records {
		items = append(items, evidenceRecordToMap(rec))
	}

	a.cacheSet(c, cacheKey, items, 30*time.Second)
	c.JSON(http.StatusOK, items)
}

func (a *app) createEvidence(c *gin.Context) {
	claims := getClaims(c)
	if !a.assertAdmin(c, claims) {
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

	req.RunName = strings.TrimSpace(req.RunName)
	req.ModelVersion = strings.TrimSpace(req.ModelVersion)
	req.DatasetName = strings.TrimSpace(req.DatasetName)
	if req.RunName == "" || req.ModelVersion == "" || req.DatasetName == "" || req.SampleCount <= 0 || req.SiteCount <= 0 || req.PositiveCount < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid evidence payload"})
		return
	}

	if req.StageRecommendation == "" {
		req.StageRecommendation = "RESEARCH_ONLY"
	}
	if req.RegulatoryStatus == "" {
		req.RegulatoryStatus = "NOT_SUBMITTED"
	}
	if !containsString(deploymentStages, req.StageRecommendation) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid stageRecommendation"})
		return
	}
	if !containsString(regulatoryStatuses, req.RegulatoryStatus) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid regulatoryStatus"})
		return
	}

	rec := &domain.ClinicalEvidenceRun{
		RunName:                  req.RunName,
		ModelVersion:             req.ModelVersion,
		DatasetName:              req.DatasetName,
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
		CreatedByUserID:          &claims.UserID,
	}
	if err := a.opsService.CreateEvidence(c.Request.Context(), rec); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create evidence"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
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
	})
	a.cacheInvalidatePrefixes(c, "ops:evidence:", "ops:dashboard:")
}

func (a *app) listIncidents(c *gin.Context) {
	claims := getClaims(c)
	if !a.assertAdmin(c, claims) {
		return
	}

	status := strings.TrimSpace(c.Query("status"))
	limit := parseLimit(c, 30)
	if status != "" && !containsString(incidentStatuses, status) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
		return
	}
	cacheKey := fmt.Sprintf("ops:incidents:list:v1:status=%s:limit=%d", status, limit)
	var cached []map[string]any
	if a.cacheGet(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	incidents, err := a.opsService.ListIncidents(c.Request.Context(), status, limit)
	if err != nil {
		if errors.Is(err, service.ErrInvalidIncidentStatus) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid status"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list incidents"})
		return
	}

	items := make([]map[string]any, 0, limit)
	for _, i := range incidents {

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

	a.cacheSet(c, cacheKey, items, 15*time.Second)
	c.JSON(http.StatusOK, items)
}

func (a *app) createIncident(c *gin.Context) {
	claims := getClaims(c)
	if !a.assertAdmin(c, claims) {
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

	req.Source = strings.TrimSpace(req.Source)
	req.Severity = strings.TrimSpace(req.Severity)
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || !containsString(incidentSources, req.Source) || !containsString(incidentSeverities, req.Severity) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid incident payload"})
		return
	}

	incident, err := a.opsService.CreateIncident(c.Request.Context(), service.CreateIncidentInput{
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
		"id":             incident.ID,
		"source":         incident.Source,
		"severity":       incident.Severity,
		"status":         incident.Status,
		"title":          incident.Title,
		"detail":         strOrNil(incident.Detail),
		"ownerUserId":    strOrNil(incident.OwnerUserID),
		"openedAt":       incident.OpenedAt,
		"acknowledgedAt": incident.AcknowledgedAt,
		"resolvedAt":     incident.ResolvedAt,
		"createdAt":      incident.CreatedAt,
		"updatedAt":      incident.UpdatedAt,
	})
	a.cacheInvalidatePrefixes(c, "ops:incidents:", "ops:dashboard:")
}

func (a *app) transitionIncident(c *gin.Context) {
	claims := getClaims(c)
	if !a.assertAdmin(c, claims) {
		return
	}

	incidentID := strings.TrimSpace(c.Param("id"))
	var req struct {
		Action string `json:"action"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	req.Action = strings.TrimSpace(req.Action)
	if incidentID == "" || !containsString(incidentTransitions, req.Action) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid incident transition"})
		return
	}

	incident, existingStatus, err := a.opsService.TransitionIncident(c.Request.Context(), incidentID, req.Action, claims.UserID)
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

	a.writeAudit(c, claims, "OPS_INCIDENT_TRANSITION", "ops_incident", incident.ID, "SUCCESS", map[string]any{"action": req.Action, "fromStatus": existingStatus, "toStatus": incident.Status})

	c.JSON(http.StatusOK, gin.H{
		"id":             incident.ID,
		"source":         incident.Source,
		"severity":       incident.Severity,
		"status":         incident.Status,
		"title":          incident.Title,
		"detail":         strOrNil(incident.Detail),
		"ownerUserId":    strOrNil(incident.OwnerUserID),
		"openedAt":       incident.OpenedAt,
		"acknowledgedAt": incident.AcknowledgedAt,
		"resolvedAt":     incident.ResolvedAt,
		"createdAt":      incident.CreatedAt,
		"updatedAt":      incident.UpdatedAt,
	})
	a.cacheInvalidatePrefixes(c, "ops:incidents:", "ops:dashboard:")
}
