package main

// 文件： cmd/server/user_compliance_handlers.go
// 用途：网关的处理器、中间件与对外 HTTP API 路由装配。

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"cancer-detection-backend/internal/domain"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var clinicalScope = gin.H{
	"indication": "胸部X光肺部癌症辅助筛查",
	"outOfScope": []string{
		"非胸部X光影像",
		"儿科病例",
		"急诊替代诊断",
		"仅凭AI结果出具最终诊断",
	},
	"highRiskThreshold":  0.75,
	"productPositioning": "仅作医生辅助决策，不可替代临床诊断",
}

func (a *app) getImageByID(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid image id"})
		return
	}

	var image domain.Image
	err := a.orm.WithContext(c.Request.Context()).
		Model(&domain.Image{}).
		Preload("Patient.User").
		Preload("Detections", func(db *gorm.DB) *gorm.DB {
			return db.Preload("Reviewer").Order("created_at DESC")
		}).
		Where("id = ?", id).
		First(&image).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "Image not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to load image"})
		return
	}

	if claims.Role == "PATIENT" && image.UploadedBy != claims.UserID {
		c.JSON(http.StatusForbidden, gin.H{"error": "Unauthorized"})
		return
	}

	detections := make([]map[string]any, 0, len(image.Detections))
	for _, d := range image.Detections {
		var reviewer any = nil
		if d.Reviewer != nil {
			reviewer = gin.H{
				"name":  d.Reviewer.Name,
				"email": d.Reviewer.Email,
			}
		}
		detections = append(detections, gin.H{
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

	c.JSON(http.StatusOK, gin.H{
		"id":           image.ID,
		"patientId":    image.PatientID,
		"filePath":     imageAccessPath(image.ID),
		"fileType":     image.FileType,
		"originalName": image.OriginalName,
		"fileSize":     image.FileSize,
		"status":       image.Status,
		"uploadedBy":   image.UploadedBy,
		"createdAt":    image.CreatedAt,
		"updatedAt":    image.UpdatedAt,
		"patient": gin.H{
			"id":          image.Patient.ID,
			"dateOfBirth": image.Patient.DateOfBirth,
			"gender":      image.Patient.Gender,
			"user": gin.H{
				"id":        image.Patient.User.ID,
				"email":     image.Patient.User.Email,
				"name":      image.Patient.User.Name,
				"role":      image.Patient.User.Role,
				"phone":     strOrNil(image.Patient.User.Phone),
				"createdAt": image.Patient.User.CreatedAt,
				"updatedAt": image.Patient.User.UpdatedAt,
			},
		},
		"detections": detections,
	})
}

func (a *app) getMe(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var user domain.User
	if err := a.orm.WithContext(c.Request.Context()).Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query user"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":        user.ID,
		"email":     user.Email,
		"name":      user.Name,
		"role":      user.Role,
		"phone":     strOrNil(user.Phone),
		"createdAt": user.CreatedAt,
	})
}

func (a *app) updateMe(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		Name  *string `json:"name"`
		Phone *string `json:"phone"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	updates := map[string]any{
		"updated_at": time.Now().UTC(),
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty"})
			return
		}
		updates["name"] = name
	}
	if req.Phone != nil {
		phone := strings.TrimSpace(*req.Phone)
		if phone == "" {
			updates["phone"] = nil
		} else {
			updates["phone"] = phone
		}
	}
	if len(updates) == 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No fields to update"})
		return
	}

	if err := a.orm.WithContext(c.Request.Context()).Model(&domain.User{}).Where("id = ?", claims.UserID).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update user"})
		return
	}

	a.getMe(c)
}

func (a *app) clinicalScopeHandler(c *gin.Context) {
	if getClaims(c) == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	c.JSON(http.StatusOK, clinicalScope)
}

func (a *app) acceptConsent(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var req struct {
		ConsentType    string `json:"consentType"`
		ConsentVersion string `json:"consentVersion"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	req.ConsentType = strings.TrimSpace(req.ConsentType)
	req.ConsentVersion = strings.TrimSpace(req.ConsentVersion)
	if req.ConsentType == "" {
		req.ConsentType = "AI_ANALYSIS"
	}
	if req.ConsentVersion == "" {
		req.ConsentVersion = "v1.0"
	}
	if req.ConsentType != "AI_ANALYSIS" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported consent type"})
		return
	}

	patientID, err := a.opsService.FindOrCreatePatient(c.Request.Context(), claims.UserID, claims.Role)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	now := time.Now().UTC()
	row := &domain.PatientConsent{
		PatientID:        patientID,
		ConsentType:      req.ConsentType,
		ConsentVersion:   req.ConsentVersion,
		Accepted:         true,
		AcceptedAt:       now,
		AcceptedByUserID: claims.UserID,
		UpdatedAt:        now,
	}
	if err := a.opsService.UpsertConsent(c.Request.Context(), row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save consent"})
		return
	}

	var saved domain.PatientConsent
	if err := a.orm.WithContext(c.Request.Context()).
		Where("patient_id = ? AND consent_type = ? AND consent_version = ?", patientID, req.ConsentType, req.ConsentVersion).
		First(&saved).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query consent"})
		return
	}

	a.writeAudit(c, claims, "CONSENT_ACCEPT", "patient_consent", saved.ID, "SUCCESS", map[string]any{
		"consentType":    req.ConsentType,
		"consentVersion": req.ConsentVersion,
	})

	c.JSON(http.StatusOK, gin.H{
		"id":               saved.ID,
		"patientId":        saved.PatientID,
		"consentType":      saved.ConsentType,
		"consentVersion":   saved.ConsentVersion,
		"accepted":         saved.Accepted,
		"acceptedAt":       saved.AcceptedAt,
		"acceptedByUserId": saved.AcceptedByUserID,
		"createdAt":        saved.CreatedAt,
		"updatedAt":        saved.UpdatedAt,
	})
}

func (a *app) latestConsent(c *gin.Context) {
	claims := getClaims(c)
	if claims == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	consentType := strings.TrimSpace(c.DefaultQuery("consentType", "AI_ANALYSIS"))
	var patient domain.Patient
	if err := a.orm.WithContext(c.Request.Context()).Where("user_id = ?", claims.UserID).First(&patient).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, nil)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query patient"})
		return
	}

	var consent domain.PatientConsent
	err := a.orm.WithContext(c.Request.Context()).
		Where("patient_id = ? AND consent_type = ? AND accepted = ?", patient.ID, consentType, true).
		Order("accepted_at DESC").
		First(&consent).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusOK, nil)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to query consent"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"id":               consent.ID,
		"patientId":        consent.PatientID,
		"consentType":      consent.ConsentType,
		"consentVersion":   consent.ConsentVersion,
		"accepted":         consent.Accepted,
		"acceptedAt":       consent.AcceptedAt,
		"acceptedByUserId": consent.AcceptedByUserID,
		"createdAt":        consent.CreatedAt,
		"updatedAt":        consent.UpdatedAt,
	})
}
