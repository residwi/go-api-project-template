package user

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type StatusSnapshot struct {
	Active       bool
	TokenVersion int
}

// StatusCache speaks users and snapshots, not keys and bytes, so the service
// never learns where the value is stored.
type StatusCache interface {
	Get(ctx context.Context, userID uuid.UUID) (StatusSnapshot, bool, error)
	Put(ctx context.Context, userID uuid.UUID, snap StatusSnapshot, ttl time.Duration) error
	Invalidate(ctx context.Context, userID uuid.UUID) error
}

// NoCache always misses, so CheckStatus reads through to the repository.
type NoCache struct{}

func (NoCache) Get(context.Context, uuid.UUID) (StatusSnapshot, bool, error) {
	return StatusSnapshot{}, false, nil
}

func (NoCache) Put(context.Context, uuid.UUID, StatusSnapshot, time.Duration) error { return nil }

func (NoCache) Invalidate(context.Context, uuid.UUID) error { return nil }

var _ StatusCache = NoCache{}
