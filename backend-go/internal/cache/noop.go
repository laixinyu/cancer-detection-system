package cache

// 文件： internal/cache/noop.go
// 用途：缓存抽象接口与缓存后端实现。

import (
	"context"
	"time"
)

type NoopCache struct{}

func NewNoop() *NoopCache {
	return &NoopCache{}
}

func (n *NoopCache) Get(context.Context, string) ([]byte, bool, error) {
	return nil, false, nil
}

func (n *NoopCache) Set(context.Context, string, []byte, time.Duration) error {
	return nil
}

func (n *NoopCache) Delete(context.Context, string) error {
	return nil
}

func (n *NoopCache) DeleteByPrefix(context.Context, string) error {
	return nil
}
