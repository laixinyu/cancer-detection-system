package resilience

// 文件： internal/resilience/circuit_breaker.go
// 用途：弹性基础组件，使用熔断器保护上游依赖。

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type breakerState string

const (
	stateClosed   breakerState = "closed"
	stateOpen     breakerState = "open"
	stateHalfOpen breakerState = "half_open"
)

type CircuitBreaker struct {
	mu sync.Mutex

	failureThreshold int
	openTimeout      time.Duration
	halfOpenMaxCalls int

	state              breakerState
	consecutiveFailure int
	openedAt           time.Time
	halfOpenCalls      int
}

func NewCircuitBreaker(failureThreshold int, openTimeout time.Duration, halfOpenMaxCalls int) *CircuitBreaker {
	if failureThreshold <= 0 {
		failureThreshold = 5
	}
	if openTimeout <= 0 {
		openTimeout = 10 * time.Second
	}
	if halfOpenMaxCalls <= 0 {
		halfOpenMaxCalls = 2
	}
	return &CircuitBreaker{
		failureThreshold: failureThreshold,
		openTimeout:      openTimeout,
		halfOpenMaxCalls: halfOpenMaxCalls,
		state:            stateClosed,
	}
}

func (b *CircuitBreaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case stateClosed:
		return nil
	case stateOpen:
		if time.Since(b.openedAt) >= b.openTimeout {
			b.state = stateHalfOpen
			b.halfOpenCalls = 0
		} else {
			return ErrCircuitOpen
		}
	}

	if b.state == stateHalfOpen {
		if b.halfOpenCalls >= b.halfOpenMaxCalls {
			return ErrCircuitOpen
		}
		b.halfOpenCalls++
	}
	return nil
}

func (b *CircuitBreaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.state = stateClosed
	b.consecutiveFailure = 0
	b.openedAt = time.Time{}
	b.halfOpenCalls = 0
}

func (b *CircuitBreaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case stateHalfOpen:
		b.toOpen()
		return
	case stateOpen:
		return
	}

	b.consecutiveFailure++
	if b.consecutiveFailure >= b.failureThreshold {
		b.toOpen()
	}
}

func (b *CircuitBreaker) State() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.state)
}

func (b *CircuitBreaker) toOpen() {
	b.state = stateOpen
	b.openedAt = time.Now()
	b.consecutiveFailure = 0
	b.halfOpenCalls = 0
}

type BreakerGroup struct {
	mu       sync.Mutex
	breakers map[string]*CircuitBreaker
	failure  int
	open     time.Duration
	halfOpen int
}

func NewBreakerGroup(failureThreshold int, openTimeout time.Duration, halfOpenMaxCalls int) *BreakerGroup {
	return &BreakerGroup{
		breakers: make(map[string]*CircuitBreaker),
		failure:  failureThreshold,
		open:     openTimeout,
		halfOpen: halfOpenMaxCalls,
	}
}

func (g *BreakerGroup) For(key string) *CircuitBreaker {
	g.mu.Lock()
	defer g.mu.Unlock()
	if b, ok := g.breakers[key]; ok {
		return b
	}
	b := NewCircuitBreaker(g.failure, g.open, g.halfOpen)
	g.breakers[key] = b
	return b
}
