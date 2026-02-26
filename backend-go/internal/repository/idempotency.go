package repository

import (
	"context"
	"errors"
	"time"

	"cancer-detection-backend/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var ErrIdempotencyConflict = errors.New("idempotency key conflict")

type IdempotencyRepository interface {
	Get(ctx context.Context, scope, key string) (*domain.IdempotencyRecord, error)
	CreatePending(ctx context.Context, scope, key, requestHash string, expiresAt time.Time) (*domain.IdempotencyRecord, bool, error)
	SaveResponse(ctx context.Context, id string, status int, body []byte) error
	Delete(ctx context.Context, id string) error
}

type gormIdempotencyRepository struct {
	db *gorm.DB
}

func NewGormIdempotencyRepository(db *gorm.DB) IdempotencyRepository {
	return &gormIdempotencyRepository{db: db}
}

func (r *gormIdempotencyRepository) Get(ctx context.Context, scope, key string) (*domain.IdempotencyRecord, error) {
	var rec domain.IdempotencyRecord
	if err := r.db.WithContext(ctx).
		Where("scope = ? AND key = ? AND expires_at > ?", scope, key, time.Now().UTC()).
		First(&rec).Error; err != nil {
		return nil, err
	}
	return &rec, nil
}

func (r *gormIdempotencyRepository) CreatePending(ctx context.Context, scope, key, requestHash string, expiresAt time.Time) (*domain.IdempotencyRecord, bool, error) {
	now := time.Now().UTC()
	record := &domain.IdempotencyRecord{
		ID:          uuid.NewString(),
		Scope:       scope,
		Key:         key,
		RequestHash: requestHash,
		ExpiresAt:   expiresAt,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	result := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "scope"}, {Name: "key"}},
			DoNothing: true,
		}).
		Create(record)
	if result.Error != nil {
		return nil, false, result.Error
	}
	return record, result.RowsAffected > 0, nil
}

func (r *gormIdempotencyRepository) SaveResponse(ctx context.Context, id string, status int, body []byte) error {
	return r.db.WithContext(ctx).
		Model(&domain.IdempotencyRecord{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"response_status": status,
			"response_body":   body,
			"updated_at":      time.Now().UTC(),
		}).Error
}

func (r *gormIdempotencyRepository) Delete(ctx context.Context, id string) error {
	return r.db.WithContext(ctx).Delete(&domain.IdempotencyRecord{}, "id = ?", id).Error
}
