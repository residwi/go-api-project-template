package expire

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// Repository is expire's own storage. Its only implementation is
// expire/postgres, constructed in order/module.go. It never writes status --
// that goes through TransitionApplier -- so it only reads.
type Repository interface {
	GetExpiredOrders(ctx context.Context, limit int) ([]domain.Order, error)
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]domain.Item, error)
}
