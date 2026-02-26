package main

import (
	"context"
	"log"
	"time"

	"cancer-detection-backend/internal/events"
	"cancer-detection-backend/internal/service"
)

func startEventConsumer(ctx context.Context, auditSvc *service.AuditService) {
	if !parseBoolEnv("EVENT_CONSUMER_ENABLED", false) {
		return
	}
	consumer, err := events.NewRedisStreamConsumer(
		envOr("EVENT_BUS_REDIS_ADDR", "localhost:6379"),
		envOr("EVENT_BUS_REDIS_PASSWORD", ""),
		parseIntEnv("EVENT_BUS_REDIS_DB", 0),
		envOr("EVENT_BUS_REDIS_STREAM", "domain_events"),
		envOr("EVENT_CONSUMER_GROUP", "governance"),
		envOr("EVENT_CONSUMER_NAME", "governance-1"),
	)
	if err != nil {
		log.Printf("event consumer init failed: %v", err)
		return
	}

	go func() {
		defer consumer.Close()
		err := consumer.Run(ctx, func(ctx context.Context, message events.Message) error {
			return auditSvc.WriteAudit(ctx, service.WriteAuditInput{
				Action:     "DOMAIN_EVENT_CONSUMED",
				EntityType: message.AggregateType,
				EntityID:   message.AggregateID,
				Result:     "SUCCESS",
				Metadata: map[string]any{
					"eventId":    message.ID,
					"eventType":  message.EventType,
					"version":    message.Version,
					"occurredAt": message.OccurredAt.Format(time.RFC3339Nano),
				},
			})
		})
		if err != nil {
			log.Printf("event consumer stopped: %v", err)
		}
	}()
}
