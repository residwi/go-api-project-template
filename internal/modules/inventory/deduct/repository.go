package deduct

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

// Repository is deduct's own storage. Its only implementation is
// deduct/postgres, constructed in inventory/module.go.
type Repository interface {
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
	// Deduct has no caller: payment/charge and order always deduct a whole
	// order's items in one call, through DeductBatch. Carried rather than
	// dropped -- see Command.Deduct.
	Deduct(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
}
