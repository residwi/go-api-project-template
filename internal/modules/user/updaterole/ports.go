package updaterole

import (
	"context"

	"github.com/google/uuid"
)

// StatusInvalidator invalidates the cached account status for a user.
// Satisfied directly by query's cache -- query.StatusCache already declares
// Invalidate, so no adapter is needed. updaterole never imports query, its
// sibling slice: user/module.go wires the same cache instance to both by
// name-match.
type StatusInvalidator interface {
	Invalidate(ctx context.Context, userID uuid.UUID) error
}
