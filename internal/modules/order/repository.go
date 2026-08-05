package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type AdminListParams struct {
	paging.OffsetPage

	Status string
}

// Repository is order's persistence port. The Postgres implementation lives
// in the postgres subpackage; this package never imports it.
type Repository interface {
	Create(ctx context.Context, order *Order) error
	CreateItems(ctx context.Context, items []Item) error
	GetByID(ctx context.Context, id uuid.UUID) (*Order, error)
	GetByUserIDAndIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*Order, error)
	ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]Order, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]Order, int, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, fromStatus, toStatus Status) error
	Apply(ctx context.Context, id uuid.UUID, t Transition) error
	UpdateTotals(ctx context.Context, id uuid.UUID, discount, total int64) error
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]Item, error)
	GetExpiredOrders(ctx context.Context, limit int) ([]Order, error)
	GetStaleProcessingOrders(ctx context.Context, threshold time.Duration, limit int) ([]Order, error)
	HasDeliveredOrder(ctx context.Context, p DeliveredPurchaseParams) (bool, error)
}
