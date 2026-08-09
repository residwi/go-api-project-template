package reserve

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

// Repository is reserve's own storage. Its only implementation is
// reserve/postgres, constructed in inventory/module.go.
type Repository interface {
	ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error
	// Reserve has no caller: order/place always reserves a whole cart in one
	// call, through ReserveBatch. Carried rather than dropped -- see
	// Command.Reserve.
	Reserve(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
}
