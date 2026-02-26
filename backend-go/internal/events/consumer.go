package events

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Handler func(ctx context.Context, message Message) error

type RedisStreamConsumer struct {
	client   *redis.Client
	stream   string
	group    string
	consumer string
	block    time.Duration
}

func NewRedisStreamConsumer(addr, password string, db int, stream, group, consumer string) (*RedisStreamConsumer, error) {
	if stream == "" {
		stream = "domain_events"
	}
	if group == "" {
		group = "governance"
	}
	if consumer == "" {
		consumer = "governance-1"
	}
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis consumer ping failed: %w", err)
	}
	if err := client.XGroupCreateMkStream(ctx, stream, group, "$").Err(); err != nil && err.Error() != "BUSYGROUP Consumer Group name already exists" {
		_ = client.Close()
		return nil, err
	}
	return &RedisStreamConsumer{
		client:   client,
		stream:   stream,
		group:    group,
		consumer: consumer,
		block:    2 * time.Second,
	}, nil
}

func (c *RedisStreamConsumer) Run(ctx context.Context, handler Handler) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		rows, err := c.client.XReadGroup(ctx, &redis.XReadGroupArgs{
			Group:    c.group,
			Consumer: c.consumer,
			Streams:  []string{c.stream, ">"},
			Count:    20,
			Block:    c.block,
		}).Result()
		if err == redis.Nil {
			continue
		}
		if err != nil {
			return err
		}
		for _, stream := range rows {
			for _, message := range stream.Messages {
				raw, _ := message.Values["payload"].(string)
				event := Message{}
				if err := json.Unmarshal([]byte(raw), &event); err != nil {
					_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
					continue
				}
				if err := handler(ctx, event); err != nil {
					continue
				}
				_ = c.client.XAck(ctx, c.stream, c.group, message.ID).Err()
			}
		}
	}
}

func (c *RedisStreamConsumer) Close() error {
	if c.client == nil {
		return nil
	}
	return c.client.Close()
}
