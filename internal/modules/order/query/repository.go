package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type AdminListParams struct {
	paging.OffsetPage

	Status string
}

// DeliveredPurchaseParams names its fields because all three ids are
// uuid.UUID: a positional swap would compile and answer about the wrong
// purchase. This is query's own repository parameter, not a seam type: review
// crosses the seam with three plain uuid.UUID arguments instead.
type DeliveredPurchaseParams struct {
	UserID    uuid.UUID
	OrderID   uuid.UUID
	ProductID uuid.UUID
}

// Repository is query's own storage. Its only implementation is
// query/postgres, constructed in order/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]domain.Item, error)
	ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Order, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Order, int, error)
	HasDeliveredOrder(ctx context.Context, p DeliveredPurchaseParams) (bool, error)
}
