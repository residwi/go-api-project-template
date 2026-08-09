package transition

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// Repository is transition's own storage: the only place order's status column
// is written. Its only implementation is transition/postgres, constructed in
// order/module.go.
type Repository interface {
	// Apply is the guarded compare-and-set behind every named domain.Transition.
	Apply(ctx context.Context, id uuid.UUID, t domain.Transition) error
	// UpdateStatus is changestatus's dynamic from/to write for the handful of
	// side-effect-free statuses an admin may set directly. It is a second,
	// simpler compare-and-set (no stock flags) rather than a named Transition,
	// because changestatus's "to" is whatever the caller asked for.
	UpdateStatus(ctx context.Context, id uuid.UUID, from, to domain.Status) error
}
