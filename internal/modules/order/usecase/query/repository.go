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

type DeliveredPurchaseParams struct {
	UserID    uuid.UUID
	OrderID   uuid.UUID
	ProductID uuid.UUID
}

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]domain.Item, error)
	ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Order, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Order, int, error)
	HasDeliveredOrder(ctx context.Context, p DeliveredPurchaseParams) (bool, error)
}
