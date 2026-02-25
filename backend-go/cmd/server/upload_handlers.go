package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

func (a *app) listImages(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	limit := parseLimit(c, 20)
	cacheKey := "image:list:v1:uid=" + claims.UserID + ":role=" + claims.Role + ":limit=" + strconv.Itoa(limit)
	var cached map[string]any
	if a.cacheGet(c, cacheKey, &cached) {
		c.JSON(http.StatusOK, cached)
		return
	}

	query := a.orm.WithContext(c.Request.Context()).
		Model(&ormImage{}).
		Preload("Detections", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at DESC")
		}).
		Order("created_at DESC").
		Limit(limit + 1)
	if claims.Role == "PATIENT" {
		query = query.Where("uploaded_by = ?", claims.UserID)
	}
	var images []ormImage
	if err := query.Find(&images).Error; err != nil {
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
	if len(items) > limit {
		nextCursor = items[len(items)-1]["id"]
		items = items[:len(items)-1]
	}
	response := gin.H{"images": items, "nextCursor": nextCursor}
	a.cacheSet(c, cacheKey, response, 15*time.Second)
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
	patientID, err := a.findOrCreatePatient(ctx, claims.UserID, claims.Role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if claims.Role == "PATIENT" && !consentAccepted {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Consent required before upload"})
		return
	}

	if claims.Role == "PATIENT" && consentAccepted {
		consent := ormPatientConsent{
			PatientID:        patientID,
			ConsentType:      "AI_ANALYSIS",
			ConsentVersion:   consentVersion,
			Accepted:         true,
			AcceptedAt:       time.Now().UTC(),
			AcceptedByUserID: claims.UserID,
			UpdatedAt:        time.Now().UTC(),
		}
		if err := a.orm.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "patient_id"}, {Name: "consent_type"}, {Name: "consent_version"}},
			DoUpdates: clause.AssignmentColumns([]string{"accepted", "accepted_at", "accepted_by_user_id", "updated_at"}),
		}).Create(&consent).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save consent"})
			return
		}
	}

	fileName := fmt.Sprintf("%d_%s%s", nowMillis(), uuid.NewString(), normalizeExt(path.Ext(fileHeader.Filename)))
	absolute := filepath.Join(a.uploadDir, fileName)
	if err := os.WriteFile(absolute, data, 0o644); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save upload"})
		return
	}
	storedPath := a.uploadPrefix + "/" + fileName

	imageRecord := ormImage{
		PatientID:    patientID,
		FilePath:     storedPath,
		FileType:     fileTypeEnum(fileType),
		OriginalName: fileHeader.Filename,
		FileSize:     len(data),
		Status:       "PROCESSING",
		UploadedBy:   claims.UserID,
	}
	if err := a.orm.WithContext(ctx).Create(&imageRecord).Error; err != nil {
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
		_ = a.orm.WithContext(ctx).Model(&ormImage{}).Where("id = ?", img.ID).Updates(map[string]any{
			"status":     "FAILED",
			"updated_at": time.Now().UTC(),
		}).Error
		img.Status = "FAILED"
		a.cacheInvalidatePrefixes(c, "image:list:", "detection:list:", "analytics:")
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

	detection := ormDetection{
		ImageID:           img.ID,
		ModelVersion:      aiResult.ModelVersion,
		CancerProbability: aiResult.CancerProbability,
		Findings:          findingsBytes,
		HeatmapPath:       aiResult.HeatmapPath,
		Status:            "PENDING",
	}
	if err := a.orm.WithContext(ctx).Create(&detection).Error; err != nil {
		_ = a.orm.WithContext(ctx).Model(&ormImage{}).Where("id = ?", img.ID).Updates(map[string]any{
			"status":     "FAILED",
			"updated_at": time.Now().UTC(),
		}).Error
		img.Status = "FAILED"
		a.cacheInvalidatePrefixes(c, "image:list:", "detection:list:", "analytics:")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save detection"})
		return
	}

	if err := a.orm.WithContext(ctx).Model(&ormImage{}).Where("id = ?", img.ID).Updates(map[string]any{
		"status":     "COMPLETED",
		"updated_at": time.Now().UTC(),
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to finalize image"})
		return
	}
	img.Status = "COMPLETED"
	img.FilePath = "/api/images/" + img.ID + "/file"
	a.cacheInvalidatePrefixes(c, "image:list:", "detection:list:", "report:list:", "analytics:")

	c.JSON(http.StatusOK, gin.H{"success": true, "image": img})
}

func (a *app) imageFile(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id := c.Param("id")
	var img ormImage
	if err := a.orm.WithContext(c.Request.Context()).
		Select("id", "file_path", "original_name", "uploaded_by").
		Where("id = ?", id).
		First(&img).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
		return
	}

	if claims.Role == "PATIENT" && img.UploadedBy != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Forbidden"})
		return
	}

	if !strings.HasPrefix(img.FilePath, a.uploadPrefix+"/") {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Unsupported storage path"})
		return
	}

	rel := strings.TrimPrefix(img.FilePath, a.uploadPrefix+"/")
	absolute := filepath.Join(a.uploadDir, rel)
	blob, err := os.ReadFile(absolute)
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "File unavailable"})
		return
	}

	c.Header("Cache-Control", "private, max-age=60")
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", urlEscapeFilename(img.OriginalName)))
	c.Data(http.StatusOK, mimeByExt(path.Ext(img.OriginalName)), blob)
}

func (a *app) callAI(fileName string, data []byte) (*aiResponse, error) {
	a.metrics.IncAIRequest()
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
		a.metrics.IncAIFailure()
		a.logger.Warn("alert_ai_call_failed", "error", err.Error(), "alert_code", "AI_UNREACHABLE")
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		a.metrics.IncAIFailure()
		a.logger.Warn("alert_ai_call_failed", "status", resp.StatusCode, "alert_code", "AI_BAD_STATUS")
		return nil, fmt.Errorf("AI service request failed: %d %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var result aiResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		a.metrics.IncAIFailure()
		a.logger.Warn("alert_ai_call_failed", "error", "invalid_payload", "alert_code", "AI_INVALID_PAYLOAD")
		return nil, fmt.Errorf("invalid AI payload")
	}
	if result.ModelVersion == "" {
		a.metrics.IncAIFailure()
		a.logger.Warn("alert_ai_call_failed", "error", "missing_model_version", "alert_code", "AI_INVALID_PAYLOAD")
		return nil, fmt.Errorf("invalid AI payload")
	}
	return &result, nil
}

func (a *app) findOrCreatePatient(ctx context.Context, userID, role string) (string, error) {
	var patient ormPatient
	if err := a.orm.WithContext(ctx).Where("user_id = ?", userID).First(&patient).Error; err == nil {
		return patient.ID, nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return "", errors.New("Failed to query patient profile")
	}
	if role != "PATIENT" {
		return "", errors.New("Patient profile not found")
	}

	patient = ormPatient{
		UserID:      userID,
		DateOfBirth: time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC),
		Gender:      "OTHER",
	}
	if err := a.orm.WithContext(ctx).Create(&patient).Error; err != nil {
		return "", errors.New("Patient profile not found")
	}
	return patient.ID, nil
}
