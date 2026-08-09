package place

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// Repository is place's own storage. Its only implementation is
// place/postgres, constructed in order/module.go.
type Repository interface {
	Create(ctx context.Context, order *domain.Order) error
	CreateItems(ctx context.Context, items []domain.Item) error
	GetByUserIDAndIdempotencyKey(ctx context.Context, userID uuid.UUID, key string) (*domain.Order, error)
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]domain.Item, error)
	UpdateTotals(ctx context.Context, id uuid.UUID, discount, total int64) error
}
