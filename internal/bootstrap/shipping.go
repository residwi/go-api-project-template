package bootstrap

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// NewShippingService also returns the OrderProvider: the shipping routes need it
// for order-ownership checks, so the same adapter instance is reused.
func NewShippingService(repo shipping.Repository, tx database.TxRunner, orderSvc *order.Service) (*shipping.Service, shipping.OrderProvider) {
	provider := &shippingOrderProviderAdapter{svc: orderSvc}
	svc := shipping.NewService(repo, tx, provider, &shippingOrderUpdaterAdapter{svc: orderSvc})
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

// shippingOrderUpdaterAdapter maps shipping.OrderUpdater's intent methods to the
// matching named order.Transition, applied via order.Service.Apply.
type shippingOrderUpdaterAdapter struct{ svc *order.Service }

func (a *shippingOrderUpdaterAdapter) MarkShipped(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Apply(ctx, orderID, order.ShippedTransition)
}

func (a *shippingOrderUpdaterAdapter) MarkDelivered(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Apply(ctx, orderID, order.DeliveredTransition)
}
