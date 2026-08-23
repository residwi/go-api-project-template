package order

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type AdminListParams struct {
	paging.OffsetPage

	Status string
}

type DeliveredPurchaseParams struct {
	UserID    uuid.UUID
	OrderID   uuid.UUID
	ProductID uuid.UUID
}

type Repository interface {
	Create(ctx context.Context, order *domain.Order) error
	CreateItems(ctx context.Context, items []domain.Item) error
	GetByUserIDAndIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*domain.Order, error)
	UpdateTotals(ctx context.Context, id uuid.UUID, discount, total int64) error

	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]domain.Item, error)
	ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Order, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Order, int, error)
	HasDeliveredOrder(ctx context.Context, p DeliveredPurchaseParams) (bool, error)

	GetExpiredOrders(ctx context.Context, limit int) ([]domain.Order, error)
	GetStaleProcessingOrders(ctx context.Context, threshold time.Duration, limit int) ([]domain.Order, error)

	Apply(ctx context.Context, id uuid.UUID, t domain.Transition) error
	UpdateStatus(ctx context.Context, id uuid.UUID, from, to domain.Status) error
}
