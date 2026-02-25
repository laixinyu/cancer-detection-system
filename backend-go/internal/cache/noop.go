package cache

// File: internal/cache/noop.go
// Purpose: Cache abstraction interfaces and cache backend implementations.

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
