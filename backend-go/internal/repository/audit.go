package repository

import (
	"context"

	"cancer-detection-backend/internal/domain"

	"gorm.io/gorm"
)

type AuditListFilter struct {
	Action     string
	EntityType string
	EntityID   string
	Result     string
	Limit      int
}

type AuditRepository interface {
	List(ctx context.Context, filter AuditListFilter) ([]domain.AuditLog, error)
	Create(ctx context.Context, log *domain.AuditLog) error
}

type gormAuditRepository struct {
	db *gorm.DB
}

func NewGormAuditRepository(db *gorm.DB) AuditRepository {
	return &gormAuditRepository{db: db}
}

func (r *gormAuditRepository) List(ctx context.Context, filter AuditListFilter) ([]domain.AuditLog, error) {
	query := r.db.WithContext(ctx).
		Model(&domain.AuditLog{}).
		Preload("ActorUser").
		Order("created_at DESC").
		Limit(filter.Limit)

	if filter.Action != "" {
		query = query.Where("action = ?", filter.Action)
	}
	if filter.EntityType != "" {
		query = query.Where("entity_type = ?", filter.EntityType)
	}
	if filter.EntityID != "" {
		query = query.Where("entity_id = ?", filter.EntityID)
	}
	if filter.Result != "" {
		query = query.Where("result = ?", filter.Result)
	}

	var logs []domain.AuditLog
	if err := query.Find(&logs).Error; err != nil {
		return nil, err
	}
	return logs, nil
}

func (r *gormAuditRepository) Create(ctx context.Context, log *domain.AuditLog) error {
	return r.db.WithContext(ctx).Create(log).Error
}
