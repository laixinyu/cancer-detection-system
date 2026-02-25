package repository

// File: internal/repository/detection.go
// Purpose: Repository layer responsible for data access and persistence.

import (
	"context"
	"time"

	"cancer-detection-backend/internal/domain"

	"gorm.io/gorm"
)

type DetectionListFilter struct {
	Status          string
	Priority        string
	OrderByPriority bool
	PatientUserID   string
	Limit           int
}

type DetectionRepository interface {
	List(ctx context.Context, filter DetectionListFilter) ([]domain.Detection, error)
	GetByID(ctx context.Context, id string) (*domain.Detection, error)
	UpdateReview(ctx context.Context, id string, status, reviewerID string, reviewNotes *string, findings []byte, hasFindings bool) error
}

type gormDetectionRepository struct {
	db *gorm.DB
}

func NewGormDetectionRepository(db *gorm.DB) DetectionRepository {
	return &gormDetectionRepository{db: db}
}

func (r *gormDetectionRepository) List(ctx context.Context, filter DetectionListFilter) ([]domain.Detection, error) {
	q := r.db.WithContext(ctx).
		Model(&domain.Detection{}).
		Preload("Image").
		Preload("Image.Patient").
		Preload("Image.Patient.User").
		Preload("Reviewer")

	if filter.Status != "" {
		q = q.Where("detections.status = ?", filter.Status)
	}
	if filter.PatientUserID != "" {
		q = q.Joins("JOIN images ON images.id = detections.image_id").Where("images.uploaded_by = ?", filter.PatientUserID)
	}
	if filter.Priority != "" {
		switch filter.Priority {
		case "HIGH":
			q = q.Where("detections.cancer_probability >= ?", 0.7)
		case "MEDIUM":
			q = q.Where("detections.cancer_probability >= ? AND detections.cancer_probability < ?", 0.3, 0.7)
		case "LOW":
			q = q.Where("detections.cancer_probability < ?", 0.3)
		}
	}
	if filter.OrderByPriority {
		q = q.Order("detections.cancer_probability DESC").Order("detections.created_at ASC")
	} else {
		q = q.Order("detections.created_at DESC")
	}

	var rows []domain.Detection
	if err := q.Limit(filter.Limit).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *gormDetectionRepository) GetByID(ctx context.Context, id string) (*domain.Detection, error) {
	var row domain.Detection
	if err := r.db.WithContext(ctx).
		Preload("Image").
		Preload("Image.Patient").
		Preload("Image.Patient.User").
		Preload("Reviewer").
		First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormDetectionRepository) UpdateReview(ctx context.Context, id string, status, reviewerID string, reviewNotes *string, findings []byte, hasFindings bool) error {
	updates := map[string]any{
		"status":       status,
		"review_notes": reviewNotes,
		"reviewed_by":  reviewerID,
		"updated_at":   time.Now().UTC(),
	}
	if hasFindings {
		updates["findings"] = findings
	}
	return r.db.WithContext(ctx).Model(&domain.Detection{}).Where("id = ?", id).Updates(updates).Error
}
