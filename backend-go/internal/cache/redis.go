package cache

// 文件： internal/cache/redis.go
// 用途：缓存抽象接口与缓存后端实现。

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	client *redis.Client
}

const redisPrefixIndexTTL = 24 * time.Hour

func NewRedis(addr, password string, db int) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("redis ping failed: %w", err)
	}

	return &RedisCache{client: client}, nil
}

func (r *RedisCache) Get(ctx context.Context, key string) ([]byte, bool, error) {
	v, err := r.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return []byte(v), true, nil
}

func (r *RedisCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 0
	}
	prefixes := derivePrefixes(key)
	pipe := r.client.Pipeline()
	pipe.Set(ctx, key, value, ttl)
	for _, prefix := range prefixes {
		indexKey := prefixIndexKey(prefix)
		pipe.SAdd(ctx, indexKey, key)
		pipe.Expire(ctx, indexKey, redisPrefixIndexTTL)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisCache) Delete(ctx context.Context, key string) error {
	prefixes := derivePrefixes(key)
	pipe := r.client.Pipeline()
	pipe.Del(ctx, key)
	for _, prefix := range prefixes {
		pipe.SRem(ctx, prefixIndexKey(prefix), key)
	}
	_, err := pipe.Exec(ctx)
	return err
}

func (r *RedisCache) DeleteByPrefix(ctx context.Context, prefix string) error {
	if prefix == "" {
		return nil
	}

	indexKey := prefixIndexKey(prefix)
	indexedKeys, err := r.client.SMembers(ctx, indexKey).Result()
	if err != nil {
		return err
	}
	if len(indexedKeys) > 0 {
		pipe := r.client.Pipeline()
		pipe.Unlink(ctx, indexedKeys...)
		pipe.Del(ctx, indexKey)
		_, err := pipe.Exec(ctx)
		return err
	}

	cursor := uint64(0)
	pattern := prefix + "*"
	for {
		keys, nextCursor, err := r.client.Scan(ctx, cursor, pattern, 500).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := r.client.Unlink(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		cursor = nextCursor
		if cursor == 0 {
			return nil
		}
	}
}

func prefixIndexKey(prefix string) string {
	return "cache:index:prefix:" + prefix
}

func derivePrefixes(key string) []string {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	prefixes := make([]string, 0, 6)
	for i := 0; i < len(key); i++ {
		if key[i] == ':' {
			prefixes = append(prefixes, key[:i+1])
		}
	}
	return prefixes
}

func (r *RedisCache) Close() error {
	if r.client == nil {
		return nil
	}
	return r.client.Close()
}
