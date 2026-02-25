package repository

import (
	"context"
	"time"

	"cancer-detection-backend/internal/domain"

	"gorm.io/gorm"
)

type UploadRepository interface {
	ListImages(ctx context.Context, uploadedBy string, limit int) ([]domain.Image, error)
	CreateImage(ctx context.Context, row *domain.Image) error
	UpdateImageStatus(ctx context.Context, id, status string) error
	CreateDetection(ctx context.Context, row *domain.Detection) error
	GetImageByID(ctx context.Context, id string) (*domain.Image, error)
}

type gormUploadRepository struct {
	db *gorm.DB
}

func NewGormUploadRepository(db *gorm.DB) UploadRepository {
	return &gormUploadRepository{db: db}
}

func (r *gormUploadRepository) ListImages(ctx context.Context, uploadedBy string, limit int) ([]domain.Image, error) {
	q := r.db.WithContext(ctx).
		Model(&domain.Image{}).
		Preload("Detections", func(db *gorm.DB) *gorm.DB {
			return db.Order("created_at DESC")
		}).
		Order("created_at DESC").
		Limit(limit)
	if uploadedBy != "" {
		q = q.Where("uploaded_by = ?", uploadedBy)
	}
	var rows []domain.Image
	if err := q.Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *gormUploadRepository) CreateImage(ctx context.Context, row *domain.Image) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *gormUploadRepository) UpdateImageStatus(ctx context.Context, id, status string) error {
	return r.db.WithContext(ctx).Model(&domain.Image{}).Where("id = ?", id).Updates(map[string]any{
		"status":     status,
		"updated_at": time.Now().UTC(),
	}).Error
}

func (r *gormUploadRepository) CreateDetection(ctx context.Context, row *domain.Detection) error {
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *gormUploadRepository) GetImageByID(ctx context.Context, id string) (*domain.Image, error) {
	var row domain.Image
	if err := r.db.WithContext(ctx).Select("id", "file_path", "original_name", "uploaded_by").Where("id = ?", id).First(&row).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

