package cancel

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// Repository is cancel's own storage. Its only implementation is
// cancel/postgres, constructed in order/module.go. It never writes status --
// that goes through TransitionApplier -- so it only reads.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]domain.Item, error)
}
