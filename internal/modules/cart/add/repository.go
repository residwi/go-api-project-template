package add

import (
	"context"

	"github.com/google/uuid"
)

// Repository is add's own storage. Its only implementation is add/postgres,
// constructed in cart/module.go.
type Repository interface {
	GetOrCreate(ctx context.Context, userID uuid.UUID) (uuid.UUID, error)
	// CountAndHasItem answers both in one round-trip, so Execute enforces the
	// distinct-item cap without a second query.
	CountAndHasItem(ctx context.Context, cartID, productID uuid.UUID) (count int, hasProduct bool, err error)
	AddItem(ctx context.Context, cartID, productID uuid.UUID, qty int) error
}
