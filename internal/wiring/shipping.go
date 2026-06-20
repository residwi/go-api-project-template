package wiring

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/shipping"
)

// NewShippingService also returns the OrderProvider: the shipping routes need it
// for order-ownership checks, so the same adapter instance is reused.
func NewShippingService(repo shipping.Repository, pool *pgxpool.Pool, orderSvc *order.Service) (*shipping.Service, shipping.OrderProvider) {
	provider := &shippingOrderProviderAdapter{svc: orderSvc}
	svc := shipping.NewService(repo, pool, provider, &orderStatusUpdaterAdapter{svc: orderSvc})
	return svc, provider
}

type shippingOrderProviderAdapter struct{ svc *order.Service }

func (a *shippingOrderProviderAdapter) GetByID(ctx context.Context, orderID uuid.UUID) (shipping.OrderInfo, error) {
	o, err := a.svc.GetOrderByID(ctx, orderID)
	if err != nil {
		return shipping.OrderInfo{}, err
	}
	return shipping.OrderInfo{ID: o.ID, UserID: o.UserID, Status: string(o.Status)}, nil
}
