package user

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// StatusSnapshot is the auth-relevant state the middleware needs on every
// authenticated request.
type StatusSnapshot struct {
	Active       bool
	TokenVersion int
}

// StatusCache is user's caching port, the counterpart to Repository. It speaks
// users and snapshots, not keys and bytes, so the service never learns how or
// where the value is stored. The Redis implementation lives in the redis
// subpackage; this package never imports it.
type StatusCache interface {
	Get(ctx context.Context, userID uuid.UUID) (StatusSnapshot, bool, error)
	Put(ctx context.Context, userID uuid.UUID, snap StatusSnapshot, ttl time.Duration) error
	Invalidate(ctx context.Context, userID uuid.UUID) error
}

// NoCache satisfies StatusCache when no cache backend is configured. Get
// always misses, so CheckStatus reads through to the repository every time.
type NoCache struct{}

func (NoCache) Get(context.Context, uuid.UUID) (StatusSnapshot, bool, error) {
	return StatusSnapshot{}, false, nil
}

func (NoCache) Put(context.Context, uuid.UUID, StatusSnapshot, time.Duration) error { return nil }

func (NoCache) Invalidate(context.Context, uuid.UUID) error { return nil }

var _ StatusCache = NoCache{}
