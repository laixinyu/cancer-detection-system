package repository

import (
	"context"
	"time"

	"cancer-detection-backend/internal/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type OutboxRepository interface {
	Enqueue(ctx context.Context, eventType, aggregateType, aggregateID string, payload []byte, idempotencyKey *string) error
	ListPending(ctx context.Context, limit int) ([]domain.OutboxEvent, error)
	MarkPublished(ctx context.Context, id string) error
	MarkFailed(ctx context.Context, id string, lastError string, maxAttempts int) error
}

type gormOutboxRepository struct {
	db *gorm.DB
}

func NewGormOutboxRepository(db *gorm.DB) OutboxRepository {
	return &gormOutboxRepository{db: db}
}

func (r *gormOutboxRepository) Enqueue(ctx context.Context, eventType, aggregateType, aggregateID string, payload []byte, idempotencyKey *string) error {
	now := time.Now().UTC()
	row := &domain.OutboxEvent{
		ID:             uuid.NewString(),
		EventType:      eventType,
		AggregateType:  aggregateType,
		AggregateID:    aggregateID,
		Payload:        payload,
		IdempotencyKey: idempotencyKey,
		Status:         "PENDING",
		Attempts:       0,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	return r.db.WithContext(ctx).Create(row).Error
}

func (r *gormOutboxRepository) ListPending(ctx context.Context, limit int) ([]domain.OutboxEvent, error) {
	if limit <= 0 {
		limit = 50
	}
	rows := make([]domain.OutboxEvent, 0, limit)
	if err := r.db.WithContext(ctx).
		Where("status = ?", "PENDING").
		Order("created_at ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *gormOutboxRepository) MarkPublished(ctx context.Context, id string) error {
	now := time.Now().UTC()
	return r.db.WithContext(ctx).
		Model(&domain.OutboxEvent{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"status":       "PUBLISHED",
			"published_at": now,
			"updated_at":   now,
		}).Error
}

func (r *gormOutboxRepository) MarkFailed(ctx context.Context, id string, lastError string, maxAttempts int) error {
	row := &domain.OutboxEvent{}
	if err := r.db.WithContext(ctx).First(row, "id = ?", id).Error; err != nil {
		return err
	}
	row.Attempts++
	if row.Attempts >= maxAttempts {
		row.Status = "FAILED"
	}
	row.LastError = &lastError
	row.UpdatedAt = time.Now().UTC()
	return r.db.WithContext(ctx).Save(row).Error
}
