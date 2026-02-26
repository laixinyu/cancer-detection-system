package events

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"cancer-detection-backend/internal/repository"
)

type Relay struct {
	outbox      repository.OutboxRepository
	publisher   Publisher
	logger      *slog.Logger
	batch       int
	maxAttempts int
	interval    time.Duration
}

func NewRelay(outbox repository.OutboxRepository, publisher Publisher, logger *slog.Logger, batch int, maxAttempts int, interval time.Duration) *Relay {
	if batch <= 0 {
		batch = 50
	}
	if maxAttempts <= 0 {
		maxAttempts = 8
	}
	if interval <= 0 {
		interval = time.Second
	}
	return &Relay{
		outbox:      outbox,
		publisher:   publisher,
		logger:      logger,
		batch:       batch,
		maxAttempts: maxAttempts,
		interval:    interval,
	}
}

func (r *Relay) Run(ctx context.Context) {
	ticker := time.NewTicker(r.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.flush(ctx)
		}
	}
}

func (r *Relay) flush(ctx context.Context) {
	rows, err := r.outbox.ListPending(ctx, r.batch)
	if err != nil {
		r.logger.Warn("outbox_list_failed", "error", err.Error())
		return
	}
	for _, row := range rows {
		msg := Message{
			ID:             row.ID,
			Version:        "v1",
			EventType:      row.EventType,
			AggregateType:  row.AggregateType,
			AggregateID:    row.AggregateID,
			Payload:        json.RawMessage(row.Payload),
			IdempotencyKey: row.IdempotencyKey,
			OccurredAt:     row.CreatedAt,
		}
		if err := r.publisher.Publish(ctx, msg); err != nil {
			_ = r.outbox.MarkFailed(ctx, row.ID, err.Error(), r.maxAttempts)
			r.logger.Warn("outbox_publish_failed", "event_id", row.ID, "error", err.Error())
			continue
		}
		if err := r.outbox.MarkPublished(ctx, row.ID); err != nil {
			r.logger.Warn("outbox_mark_published_failed", "event_id", row.ID, "error", err.Error())
		}
	}
}
