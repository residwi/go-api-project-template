package wiring

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/features/cart"
	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
)

func NewOrderService(
	repo order.Repository,
	pool *pgxpool.Pool,
	cartSvc *cart.Service,
	inventorySvc *inventory.Service,
	promotionSvc *promotion.Service,
	notificationSvc *notification.Service,
) *order.Service {
	return order.NewService(
		repo, pool,
		&cartProviderAdapter{svc: cartSvc},
		&inventoryReserverAdapter{svc: inventorySvc},
		nil, // payment deps are circular — wired by SetOrderPaymentDeps
		nil,
		&couponReserverAdapter{svc: promotionSvc},
		&notificationEnqueuerAdapter{svc: notificationSvc},
	)
}

// SetOrderPaymentDeps breaks the order⇄payment cycle: it wires payment-backed
// deps onto the order service once the payment service exists.
func SetOrderPaymentDeps(orderSvc *order.Service, paymentSvc *payment.Service) {
	orderSvc.SetPaymentDeps(
		&paymentInitiatorAdapter{svc: paymentSvc},
		&paymentJobCancellerAdapter{svc: paymentSvc},
	)
}

type cartProviderAdapter struct{ svc *cart.Service }

func (a *cartProviderAdapter) GetCart(ctx context.Context, userID uuid.UUID) (*order.CartSnapshot, error) {
	c, err := a.svc.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	snap := &order.CartSnapshot{ID: c.ID}
	for _, item := range c.Items {
		si := order.CartSnapshotItem{
			ProductID: item.ProductID,
			Quantity:  item.Quantity,
		}
		if item.Product != nil {
			si.Name = item.Product.Name
			si.Price = item.Product.Price
			si.Currency = item.Product.Currency
			si.Status = item.Product.Status
		}
		snap.Items = append(snap.Items, si)
	}
	return snap, nil
}

func (a *cartProviderAdapter) Clear(ctx context.Context, userID uuid.UUID) error {
	return a.svc.Clear(ctx, userID)
}

type inventoryReserverAdapter struct{ svc *inventory.Service }

func (a *inventoryReserverAdapter) ReserveBatch(ctx context.Context, items []order.InventoryItem) error {
	return a.svc.ReserveBatch(ctx, orderToStockChanges(items))
}

func (a *inventoryReserverAdapter) ReleaseBatch(ctx context.Context, items []order.InventoryItem) error {
	return a.svc.ReleaseBatch(ctx, orderToStockChanges(items))
}

func orderToStockChanges(items []order.InventoryItem) []inventory.StockChange {
	changes := make([]inventory.StockChange, len(items))
	for i, it := range items {
		changes[i] = inventory.StockChange{ProductID: it.ProductID, Quantity: it.Quantity}
	}
	return changes
}

type paymentInitiatorAdapter struct{ svc *payment.Service }

func (a *paymentInitiatorAdapter) InitiatePayment(ctx context.Context, params order.InitiatePaymentParams) (order.PaymentResult, error) {
	result, err := a.svc.InitiatePayment(ctx, payment.InitiatePaymentParams{
		OrderID:         params.OrderID,
		Amount:          params.Amount,
		Currency:        params.Currency,
		PaymentMethodID: params.PaymentMethodID,
	})
	if err != nil {
		return order.PaymentResult{}, err
	}
	return order.PaymentResult{
		PaymentID:  result.PaymentID,
		PaymentURL: result.PaymentURL,
		Charged:    result.Charged,
	}, nil
}

type paymentJobCancellerAdapter struct{ svc *payment.Service }

func (a *paymentJobCancellerAdapter) CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.CancelJobsByOrderID(ctx, orderID)
}

type couponReserverAdapter struct{ svc *promotion.Service }

func (a *couponReserverAdapter) Reserve(ctx context.Context, code string, userID, orderID uuid.UUID, orderSubtotal int64) (int64, error) {
	return a.svc.Reserve(ctx, code, userID, orderID, orderSubtotal)
}

func (a *couponReserverAdapter) Release(ctx context.Context, orderID uuid.UUID) error {
	return a.svc.Release(ctx, orderID)
}

type notificationEnqueuerAdapter struct{ svc *notification.Service }

func (a *notificationEnqueuerAdapter) EnqueueOrderPlaced(ctx context.Context, userID, orderID uuid.UUID) error {
	return a.svc.EnqueueOrderPlaced(ctx, userID, orderID)
}
