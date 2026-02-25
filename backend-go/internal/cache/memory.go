package cache

// 文件： internal/cache/memory.go
// 用途：缓存抽象接口与缓存后端实现。

import (
	"context"
	"strings"
	"sync"
	"time"
)

type memoryEntry struct {
	value    []byte
	expireAt time.Time
}

type MemoryCache struct {
	mu          sync.RWMutex
	data        map[string]memoryEntry
	stopJanitor chan struct{}
}

func NewMemory() *MemoryCache {
	c := &MemoryCache{
		data:        make(map[string]memoryEntry),
		stopJanitor: make(chan struct{}),
	}
	go c.janitor()
	return c
}

func (m *MemoryCache) Close() error {
	close(m.stopJanitor)
	return nil
}

func (m *MemoryCache) Get(_ context.Context, key string) ([]byte, bool, error) {
	now := time.Now()

	m.mu.RLock()
	entry, ok := m.data[key]
	m.mu.RUnlock()
	if !ok {
		return nil, false, nil
	}
	if !entry.expireAt.IsZero() && now.After(entry.expireAt) {
		m.mu.Lock()
		delete(m.data, key)
		m.mu.Unlock()
		return nil, false, nil
	}

	out := make([]byte, len(entry.value))
	copy(out, entry.value)
	return out, true, nil
}

func (m *MemoryCache) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	expireAt := time.Time{}
	if ttl > 0 {
		expireAt = time.Now().Add(ttl)
	}

	copied := make([]byte, len(value))
	copy(copied, value)

	m.mu.Lock()
	m.data[key] = memoryEntry{
		value:    copied,
		expireAt: expireAt,
	}
	m.mu.Unlock()
	return nil
}

func (m *MemoryCache) Delete(_ context.Context, key string) error {
	m.mu.Lock()
	delete(m.data, key)
	m.mu.Unlock()
	return nil
}

func (m *MemoryCache) DeleteByPrefix(_ context.Context, prefix string) error {
	m.mu.Lock()
	for k := range m.data {
		if strings.HasPrefix(k, prefix) {
			delete(m.data, k)
		}
	}
	m.mu.Unlock()
	return nil
}

func (m *MemoryCache) janitor() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.cleanupExpired()
		case <-m.stopJanitor:
			return
		}
	}
}

func (m *MemoryCache) cleanupExpired() {
	now := time.Now()
	m.mu.Lock()
	for k, v := range m.data {
		if !v.expireAt.IsZero() && now.After(v.expireAt) {
			delete(m.data, k)
		}
	}
	m.mu.Unlock()
}
