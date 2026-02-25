package resilience

// 文件： internal/resilience/limiter.go
// 用途：弹性基础组件，包括令牌桶限流器与按键限流器。

import (
	"sync"
	"time"
)

type tokenBucket struct {
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func newTokenBucket(rate float64, burst int) *tokenBucket {
	if rate <= 0 {
		rate = 1
	}
	if burst <= 0 {
		burst = 1
	}
	return &tokenBucket{
		rate:   rate,
		burst:  float64(burst),
		tokens: float64(burst),
		last:   time.Now(),
	}
}

func (b *tokenBucket) allow(now time.Time) bool {
	elapsed := now.Sub(b.last).Seconds()
	b.last = now
	b.tokens = minFloat(b.burst, b.tokens+(elapsed*b.rate))
	if b.tokens < 1 {
		return false
	}
	b.tokens -= 1
	return true
}

type keyedLimiterEntry struct {
	bucket   *tokenBucket
	lastSeen time.Time
}

type KeyedLimiter struct {
	mu       sync.Mutex
	rate     float64
	burst    int
	ttl      time.Duration
	entries  map[string]*keyedLimiterEntry
	stopChan chan struct{}
}

func NewKeyedLimiter(rate float64, burst int, ttl time.Duration) *KeyedLimiter {
	if ttl <= 0 {
		ttl = 2 * time.Minute
	}
	l := &KeyedLimiter{
		rate:     rate,
		burst:    burst,
		ttl:      ttl,
		entries:  make(map[string]*keyedLimiterEntry),
		stopChan: make(chan struct{}),
	}
	go l.janitor()
	return l
}

func (l *KeyedLimiter) Allow(key string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[key]
	if !ok {
		e = &keyedLimiterEntry{bucket: newTokenBucket(l.rate, l.burst)}
		l.entries[key] = e
	}
	e.lastSeen = now
	return e.bucket.allow(now)
}

func (l *KeyedLimiter) Close() {
	close(l.stopChan)
}

func (l *KeyedLimiter) janitor() {
	ticker := time.NewTicker(l.ttl / 2)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.cleanup()
		case <-l.stopChan:
			return
		}
	}
}

func (l *KeyedLimiter) cleanup() {
	deadline := time.Now().Add(-l.ttl)
	l.mu.Lock()
	for key, e := range l.entries {
		if e.lastSeen.Before(deadline) {
			delete(l.entries, key)
		}
	}
	l.mu.Unlock()
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
