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

type StatusCache interface {
	Get(ctx context.Context, userID uuid.UUID) (StatusSnapshot, bool, error)
	Put(ctx context.Context, userID uuid.UUID, snap StatusSnapshot, ttl time.Duration) error
	Invalidate(ctx context.Context, userID uuid.UUID) error
}

type NoCache struct{}

func (NoCache) Get(context.Context, uuid.UUID) (StatusSnapshot, bool, error) {
	return StatusSnapshot{}, false, nil
}

func (NoCache) Put(context.Context, uuid.UUID, StatusSnapshot, time.Duration) error { return nil }

func (NoCache) Invalidate(context.Context, uuid.UUID) error { return nil }

var _ StatusCache = NoCache{}
