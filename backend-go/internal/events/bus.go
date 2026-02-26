package events

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"
)

type Message struct {
	ID             string          `json:"id"`
	Version        string          `json:"version"`
	EventType      string          `json:"eventType"`
	AggregateType  string          `json:"aggregateType"`
	AggregateID    string          `json:"aggregateId"`
	Payload        json.RawMessage `json:"payload"`
	IdempotencyKey *string         `json:"idempotencyKey,omitempty"`
	OccurredAt     time.Time       `json:"occurredAt"`
}

type Publisher interface {
	Publish(ctx context.Context, msg Message) error
}

type LogPublisher struct {
	logger *slog.Logger
}

func NewLogPublisher(logger *slog.Logger) *LogPublisher {
	return &LogPublisher{logger: logger}
}

func (p *LogPublisher) Publish(_ context.Context, msg Message) error {
	p.logger.Info("domain_event", "event_type", msg.EventType, "aggregate_type", msg.AggregateType, "aggregate_id", msg.AggregateID, "event_id", msg.ID)
	return nil
}

type RedisStreamPublisher struct {
	client *redis.Client
	stream string
}

func NewRedisStreamPublisher(addr, password string, db int, stream string) (*RedisStreamPublisher, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis stream ping failed: %w", err)
	}
	if stream == "" {
		stream = "domain_events"
	}
	return &RedisStreamPublisher{client: client, stream: stream}, nil
}

func (p *RedisStreamPublisher) Publish(ctx context.Context, msg Message) error {
	payload, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	return p.client.XAdd(ctx, &redis.XAddArgs{
		Stream: p.stream,
		Values: map[string]any{"payload": string(payload)},
	}).Err()
}

func (p *RedisStreamPublisher) Close() error {
	if p.client == nil {
		return nil
	}
	return p.client.Close()
}
