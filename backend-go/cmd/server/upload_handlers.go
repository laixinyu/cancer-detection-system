package main

// 文件： cmd/server/upload_handlers.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"path"
	"strings"
	"time"

	"cancer-detection-backend/internal/domain"
	"cancer-detection-backend/internal/resilience"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (a *app) listImages(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	limit := parseLimit(c, 20)

	images, next, err := a.uploadService.ListImages(c.Request.Context(), claims.Role, claims.UserID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to list images"})
		return
	}

	items := make([]map[string]any, 0, limit+1)
	for _, img := range images {
		detections := []any{}
		if len(img.Detections) > 0 {
			d := img.Detections[0]
			detections = append(detections, gin.H{
				"id":                d.ID,
				"createdAt":         d.CreatedAt,
				"cancerProbability": d.CancerProbability,
				"status":            d.Status,
				"modelVersion":      d.ModelVersion,
			})
		}
		items = append(items, gin.H{
			"id":           img.ID,
			"patientId":    img.PatientID,
			"filePath":     imageAccessPath(img.ID),
			"fileType":     img.FileType,
			"originalName": img.OriginalName,
			"fileSize":     img.FileSize,
			"status":       img.Status,
			"uploadedBy":   img.UploadedBy,
			"createdAt":    img.CreatedAt,
			"updatedAt":    img.UpdatedAt,
			"detections":   detections,
		})
	}
	var nextCursor any = nil
	if next != nil {
		nextCursor = *next
	}
	response := gin.H{"images": items, "nextCursor": nextCursor}
	c.JSON(http.StatusOK, response)
}

func (a *app) uploadImage(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	consentAccepted := strings.EqualFold(strings.TrimSpace(c.PostForm("consentAccepted")), "true")
	consentVersion := strings.TrimSpace(c.PostForm("consentVersion"))
	if consentVersion == "" {
		consentVersion = "v1.0"
	}

	fileHeader, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file provided"})
		return
	}
	if fileHeader.Size <= 0 || fileHeader.Size > maxUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "File too large. Maximum size is 10MB"})
		return
	}

	fileType := strings.ToUpper(strings.TrimPrefix(strings.ToLower(path.Ext(fileHeader.Filename)), "."))
	if fileType == "JPG" {
		fileType = "JPEG"
	}
	if !isAllowedExt(fileType) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file type. Only PNG, JPEG, JPG, TIFF, DCM are allowed"})
		return
	}

	opened, err := fileHeader.Open()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file"})
		return
	}
	defer opened.Close()
	data, err := io.ReadAll(io.LimitReader(opened, maxUploadBytes+1))
	if err != nil || len(data) == 0 || len(data) > maxUploadBytes {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid file payload"})
		return
	}

	ctx := c.Request.Context()
	patientID, err := a.uploadService.FindOrCreatePatient(ctx, claims.UserID, claims.Role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if claims.Role == "PATIENT" && !consentAccepted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Consent required before upload"})
		return
	}

	if claims.Role == "PATIENT" && consentAccepted {
		consent := &domain.PatientConsent{
			PatientID:        patientID,
			ConsentType:      "AI_ANALYSIS",
			ConsentVersion:   consentVersion,
			Accepted:         true,
			AcceptedAt:       time.Now().UTC(),
			AcceptedByUserID: claims.UserID,
			UpdatedAt:        time.Now().UTC(),
		}
		if err := a.uploadService.SaveConsent(ctx, consent); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save consent"})
			return
		}
	}

	fileName := fmt.Sprintf("%d_%s%s", nowMillis(), uuid.NewString(), normalizeExt(path.Ext(fileHeader.Filename)))
	storedPath := strings.TrimRight(a.uploadPrefix, "/") + "/" + fileName
	objectKey := strings.TrimPrefix(storedPath, "/")
	if err := a.objectStore.Put(ctx, objectKey, mimeByExt(path.Ext(fileHeader.Filename)), data); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to store upload"})
		return
	}

	imageRecord := &domain.Image{
		PatientID:    patientID,
		FilePath:     storedPath,
		FileType:     fileTypeEnum(fileType),
		OriginalName: fileHeader.Filename,
		FileSize:     len(data),
		Status:       "PROCESSING",
		UploadedBy:   claims.UserID,
	}
	if err := a.uploadService.CreateImage(ctx, imageRecord); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create image record"})
		return
	}
	img := imageResponse{
		ID:           imageRecord.ID,
		OriginalName: imageRecord.OriginalName,
		FilePath:     imageRecord.FilePath,
		Status:       imageRecord.Status,
	}

	aiResult, aiErr := a.callAI(fileHeader.Filename, data)
	if aiErr != nil {
		_ = a.uploadService.MarkImageFailed(ctx, img.ID)
		img.Status = "FAILED"
		a.cacheInvalidatePrefixes(c, "detection:list:", "analytics:")
		c.JSON(http.StatusBadGateway, gin.H{
			"error":   "AI detection failed",
			"details": aiErr.Error(),
			"image":   img,
		})
		return
	}

	findingsBytes, _ := json.Marshal(gin.H{
		"regions":                 aiResult.Regions,
		"labelScores":             aiResult.LabelScores,
		"infectionCoverage":       aiResult.InfectionCoverage,
		"whiteLungAssessment":     aiResult.WhiteLungAssessment,
		"topFindings":             aiResult.TopFindings,
		"decisionHighSensitivity": boolValue(aiResult.DecisionHighSensitivity),
		"decisionHighSpecificity": boolValue(aiResult.DecisionHighSpecificity),
		"taskDecisions":           aiResult.TaskDecisions,
		"operatingPointsUsed":     aiResult.OperatingPointsUsed,
		"calibrationTemperature":  aiResult.CalibrationTemperature,
		"clinicalUse":             orDefault(aiResult.ClinicalUse, "RESEARCH_ONLY"),
		"clinicalStage":           orDefault(aiResult.ClinicalStage, "RESEARCH_ONLY"),
		"detectorModelLoaded":     boolValue(aiResult.DetectorModelLoaded),
	})

	detection := &domain.Detection{
		ImageID:           img.ID,
		ModelVersion:      aiResult.ModelVersion,
		CancerProbability: aiResult.CancerProbability,
		Findings:          findingsBytes,
		HeatmapPath:       aiResult.HeatmapPath,
		Status:            "PENDING",
	}
	if err := a.uploadService.CreateDetection(ctx, detection); err != nil {
		_ = a.uploadService.MarkImageFailed(ctx, img.ID)
		img.Status = "FAILED"
		a.cacheInvalidatePrefixes(c, "detection:list:", "analytics:")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save detection"})
		return
	}

	if err := a.uploadService.MarkImageCompleted(ctx, img.ID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize image"})
		return
	}
	img.Status = "COMPLETED"
	img.FilePath = "/api/images/" + img.ID + "/file"
	a.cacheInvalidatePrefixes(c, "detection:list:", "report:list:", "analytics:")
	a.enqueueOutboxEvent(c)

	c.JSON(http.StatusOK, gin.H{"success": true, "image": img})
}

func (a *app) imageFile(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id := c.Param("id")
	img, err := a.uploadService.GetImageFileMeta(c.Request.Context(), id, claims.Role, claims.UserID)
	if err != nil {
		switch err.Error() {
		case "forbidden":
			c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		default:
			c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		}
		return
	}

	objectKey := strings.TrimPrefix(strings.TrimSpace(img.FilePath), "/")
	if objectKey == "" {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Invalid storage path"})
		return
	}
	blob, contentType, err := a.objectStore.Get(c.Request.Context(), objectKey)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "File unavailable"})
		return
	}
	if contentType == "" {
		contentType = mimeByExt(path.Ext(img.OriginalName))
	}

	c.Header("Cache-Control", "private, max-age=60")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", urlEscapeFilename(img.OriginalName)))
	c.Data(http.StatusOK, contentType, blob)
}

func (a *app) callAI(fileName string, data []byte) (*aiResponse, error) {
	a.metrics.IncAIRequest()
	if a.aiLimiter != nil && !a.aiLimiter.Allow("ai-service") {
		a.metrics.IncAIFailure()
		return nil, fmt.Errorf("ai request rate limited")
	}
	breaker := a.upstreamBreakers.For("ai-service")
	if err := breaker.Allow(); err != nil {
		a.metrics.IncAIFailure()
		if errors.Is(err, resilience.ErrCircuitOpen) {
			return nil, fmt.Errorf("ai circuit open")
		}
		return nil, err
	}
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, err
	}
	if _, err := part.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}

	req, err := http.NewRequest(http.MethodPost, a.aiService+"/predict", &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := a.httpClient.Do(req)
	if err != nil {
		breaker.RecordFailure()
		a.metrics.IncAIFailure()
		a.logger.Warn("alert_ai_call_failed", "error", err.Error(), "alert_code", "AI_UNREACHABLE")
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		breaker.RecordFailure()
		a.metrics.IncAIFailure()
		a.logger.Warn("alert_ai_call_failed", "status", resp.StatusCode, "alert_code", "AI_BAD_STATUS")
		return nil, fmt.Errorf("AI service request failed: %d %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result aiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		breaker.RecordFailure()
		a.metrics.IncAIFailure()
		a.logger.Warn("alert_ai_call_failed", "error", "invalid_payload", "alert_code", "AI_INVALID_PAYLOAD")
		return nil, fmt.Errorf("invalid AI payload")
	}
	if result.ModelVersion == "" {
		breaker.RecordFailure()
		a.metrics.IncAIFailure()
		a.logger.Warn("alert_ai_call_failed", "error", "missing_model_version", "alert_code", "AI_INVALID_PAYLOAD")
		return nil, fmt.Errorf("invalid AI payload")
	}
	breaker.RecordSuccess()
	return &result, nil
}
