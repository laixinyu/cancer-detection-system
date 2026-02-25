package repository

// File: internal/repository/report.go
// Purpose: Repository layer responsible for data access and persistence.

import (
	"context"
	"time"

	"cancer-detection-backend/internal/domain"

	"gorm.io/gorm"
)

type ReportListFilter struct {
	Status        string
	PatientID     string
	PatientUserID string
	Limit         int
}

type ReportRepository interface {
	List(ctx context.Context, filter ReportListFilter) ([]domain.Report, error)
	GetByID(ctx context.Context, id string) (*domain.Report, error)
	GetByDetectionID(ctx context.Context, detectionID string) (*domain.Report, error)
	Create(ctx context.Context, row *domain.Report) error
	Update(ctx context.Context, id string, updates map[string]any) error
	GetDetection(ctx context.Context, detectionID string) (*domain.Detection, error)
}

type gormReportRepository struct {
	db *gorm.DB
}

func NewGormReportRepository(db *gorm.DB) ReportRepository {
	return &gormReportRepository{db: db}
}

func (r *gormReportRepository) List(ctx context.Context, filter ReportListFilter) ([]domain.Report, error) {
	q := r.db.WithContext(ctx).
		Model(&domain.Report{}).
		Preload("Patient").
		Preload("Patient.User").
		Preload("Doctor").
		Preload("Detection").
		Preload("Detection.Image").
		Order("created_at DESC").
		Limit(filter.Limit)

	if filter.PatientUserID != "" {
		q = q.Joins("JOIN patients ON patients.id = reports.patient_id").Where("patients.user_id = ?", filter.PatientUserID)
	} else if filter.PatientID != "" {
		q = q.Where("reports.patient_id = ?", filter.PatientID)
	}
	if filter.Status != "" {
		q = q.Where("reports.status = ?", filter.Status)
	}

	var rows []domain.Report
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *gormReportRepository) GetByID(ctx context.Context, id string) (*domain.Report, error) {
	var row domain.Report
	if err := r.db.WithContext(ctx).
		Preload("Patient").
		Preload("Patient.User").
		Preload("Doctor").
		Preload("Detection").
		Preload("Detection.Image").
		First(&row, "reports.id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormReportRepository) GetByDetectionID(ctx context.Context, detectionID string) (*domain.Report, error) {
	var row domain.Report
	if err := r.db.WithContext(ctx).Where("detection_id = ?", detectionID).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *gormReportRepository) Create(ctx context.Context, row *domain.Report) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *gormReportRepository) Update(ctx context.Context, id string, updates map[string]any) error {
	updates["updated_at"] = time.Now().UTC()
	return r.db.WithContext(ctx).Model(&domain.Report{}).Where("id = ?", id).Updates(updates).Error
}

func (r *gormReportRepository) GetDetection(ctx context.Context, detectionID string) (*domain.Detection, error) {
	var row domain.Detection
	if err := r.db.WithContext(ctx).Preload("Image").First(&row, "id = ?", detectionID).Error; err != nil {
		return nil, err
	}
	return &row, nil
}
