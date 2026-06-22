package wiring

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
	gateway "github.com/residwi/go-api-project-template/internal/platform/payment"
)

// paymentOrderUpdaterAdapter maps payment.OrderUpdater's intent methods to the
// matching named order.Transition, applied via order.Service.Apply.
type paymentOrderUpdaterAdapter struct{ svc *order.Service }

func (a *paymentOrderUpdaterAdapter) MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Apply(ctx, orderID, order.PaymentProcessingTransition)
}

func (a *paymentOrderUpdaterAdapter) MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Apply(ctx, orderID, order.AwaitingPaymentTransition)
}

func (a *paymentOrderUpdaterAdapter) MarkPaid(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Apply(ctx, orderID, order.PaidTransition)
}

func (a *paymentOrderUpdaterAdapter) MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Apply(ctx, orderID, order.FulfillmentFailedAfterChargeTransition)
}

func (a *paymentOrderUpdaterAdapter) MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Apply(ctx, orderID, order.FulfillmentFailedCompensatingTransition)
}

func (a *paymentOrderUpdaterAdapter) MarkRefunded(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Apply(ctx, orderID, order.RefundTransition)
}

type orderGetterAdapter struct{ svc *order.Service }

func (a *orderGetterAdapter) GetByID(ctx context.Context, orderID uuid.UUID) (payment.OrderSnapshot, error) {
	o, err := a.svc.GetOrderByID(ctx, orderID)
	if err != nil {
		return payment.OrderSnapshot{}, err
	}
	couponCode := ""
	if o.CouponCode != nil {
		couponCode = *o.CouponCode
	}
	return payment.OrderSnapshot{
		TotalAmount:   o.TotalAmount,
		Currency:      o.Currency,
		Status:        string(o.Status),
		CouponCode:    couponCode,
		StockDeducted: o.StockDeducted(),
	}, nil
}

type orderItemsGetterAdapter struct{ svc *order.Service }

func (a *orderItemsGetterAdapter) ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]payment.OrderItemDTO, error) {
	items, err := a.svc.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	result := make([]payment.OrderItemDTO, len(items))
	for i, item := range items {
		result[i] = payment.OrderItemDTO{ProductID: item.ProductID, Quantity: item.Quantity}
	}
	return result, nil
}

type inventoryDeductorAdapter struct{ svc *inventory.Service }

func (a *inventoryDeductorAdapter) DeductBatch(ctx context.Context, items []payment.InventoryChange) error {
	return a.svc.DeductBatch(ctx, paymentToStockChanges(items))
}

type inventoryRestorerAdapter struct{ svc *inventory.Service }

func (a *inventoryRestorerAdapter) Restore(ctx context.Context, items []payment.InventoryChange, wasDeducted bool) error {
	return a.svc.Restore(ctx, paymentToStockChanges(items), inventoryStateFor(wasDeducted))
}

func paymentToStockChanges(items []payment.InventoryChange) []inventory.StockChange {
	changes := make([]inventory.StockChange, len(items))
	for i, it := range items {
		changes[i] = inventory.StockChange{ProductID: it.ProductID, Quantity: it.Quantity}
	}
	return changes
}

type couponReleaserAdapter struct{ svc *promotion.Service }

func (a *couponReleaserAdapter) Release(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Release(ctx, orderID)
}

func NewPaymentService(
	repo payment.Repository,
	pool *pgxpool.Pool,
	gw gateway.Gateway,
	orderSvc *order.Service,
	inventorySvc *inventory.Service,
	promotionSvc *promotion.Service,
) *payment.Service {
	return payment.NewService(
		repo, pool, gw,
		&paymentOrderUpdaterAdapter{svc: orderSvc},
		&orderGetterAdapter{svc: orderSvc},
		&orderItemsGetterAdapter{svc: orderSvc},
		&inventoryDeductorAdapter{svc: inventorySvc},
		&inventoryRestorerAdapter{svc: inventorySvc},
		&couponReleaserAdapter{svc: promotionSvc},
	)
}

// orderExpirerAdapter satisfies payment.OrderExpirer so the payment job
// processor's Sweep hook can delegate stale-order expiry to the order module.
type orderExpirerAdapter struct{ svc *order.Service }

func (a *orderExpirerAdapter) ExpireStale(ctx context.Context) error {
	return a.svc.ExpireStale(ctx)
}

// NewOrderExpirer adapts the order service to payment.OrderExpirer.
func NewOrderExpirer(orderSvc *order.Service) payment.OrderExpirer {
	return &orderExpirerAdapter{svc: orderSvc}
}
