package cache

// 文件： internal/cache/cache.go
// 用途：缓存抽象接口与缓存后端实现。

import (
	"context"
	"time"
)

type Cache interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Set(ctx context.Context, key string, value []byte, ttl time.Duration) error
	Delete(ctx context.Context, key string) error
	DeleteByPrefix(ctx context.Context, prefix string) error
}
