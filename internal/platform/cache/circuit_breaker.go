package cache

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrCircuitOpen = errors.New("redis circuit breaker open")

type CircuitBreaker struct {
	mu        sync.Mutex
	failures  int
	probing   bool
	openUntil time.Time
}

var _ redis.Limiter = (*CircuitBreaker)(nil)

func NewCircuitBreaker() *CircuitBreaker {
	return &CircuitBreaker{}
}

func (b *CircuitBreaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.openUntil.IsZero() {
		return nil
	}

	now := time.Now()
	if now.Before(b.openUntil) {
		return ErrCircuitOpen
	}

	// The probe that gets through re-arms the gate, so a cooldown expiring under
	// load releases one request instead of every goroutine waiting behind it.
	b.openUntil = now.Add(circuitCooldown)
	b.probing = true

	return nil
}

func (b *CircuitBreaker) ReportResult(result error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Any reply the server authored -- a miss, MISCONF, OOM, READONLY, a Lua error --
	// proves it is reachable, and reachability is all this breaker measures. Tripping
	// on a full disk would take reads offline while Redis still serves them fine.
	var replied redis.Error
	if result == nil || errors.As(result, &replied) {
		b.failures = 0

		// Only the probe may end a cooldown. A command allowed before the trip can
		// answer after it, and closing on that would release the whole waiting herd.
		if b.probing {
			b.openUntil = time.Time{}
			b.probing = false
		}

		return
	}

	// A caller that hung up says nothing about the server, so it neither
	// opens the breaker nor clears a real failure streak.
	if errors.Is(result, context.Canceled) {
		return
	}

	b.probing = false

	b.failures++
	if b.failures >= circuitThreshold {
		b.openUntil = time.Now().Add(circuitCooldown)
		b.failures = 0
	}
}

const (
	circuitThreshold = 5
	circuitCooldown  = 5 * time.Second
)
