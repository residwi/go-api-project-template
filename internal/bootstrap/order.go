package bootstrap

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/notification"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func NewOrderService(
	repo order.Repository,
	tx database.TxRunner,
	cartSvc *cart.Service,
	inventorySvc *inventory.Service,
	promotionSvc *promotion.Service,
	notificationSvc *notification.Service,
	log *slog.Logger,
) *order.Service {
	return order.NewService(
		repo, tx,
		&cartProviderAdapter{svc: cartSvc},
		inventorySvc, // satisfies order.InventoryReserver directly
		nil,          // payment deps are circular — wired by SetOrderPaymentDeps
		nil,
		promotionSvc,
		notificationSvc,
		log,
	)
}

// SetOrderPaymentDeps breaks the order⇄payment cycle: it wires payment-backed
// deps onto the order service once the payment service exists.
func SetOrderPaymentDeps(orderSvc *order.Service, paymentSvc *payment.Service) {
	orderSvc.SetPaymentDeps(
		&paymentInitiatorAdapter{svc: paymentSvc},
		paymentSvc, // satisfies order.PaymentJobCanceller directly
	)
}

type cartProviderAdapter struct{ svc *cart.Service }

func (a *cartProviderAdapter) LockCart(ctx context.Context, userID uuid.UUID) error {
	return a.svc.LockCart(ctx, userID)
}

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
			si.Status = item.Product.Status
		}
		snap.Items = append(snap.Items, si)
	}
	return snap, nil
}

func (a *cartProviderAdapter) Clear(ctx context.Context, userID uuid.UUID) error {
	return a.svc.Clear(ctx, userID)
}

type paymentInitiatorAdapter struct{ svc *payment.Service }

func (a *paymentInitiatorAdapter) InitiatePayment(
	ctx context.Context,
	params order.InitiatePaymentParams,
) (order.PaymentResult, error) {
	result, err := a.svc.InitiatePayment(ctx, payment.InitiatePaymentParams{
		OrderID:         params.OrderID,
		Amount:          params.Amount,
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
