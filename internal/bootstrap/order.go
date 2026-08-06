package bootstrap

import (
	"log/slog"

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
		cartSvc,      // satisfies order.CartProvider directly
		inventorySvc, // satisfies order.InventoryReserver directly
		nil,          // payment deps are circular — wired by SetOrderPaymentDeps
		nil,
		promotionSvc,
		notificationSvc,
		log,
	)
}

// SetOrderPaymentDeps breaks the order-payment cycle: at whole-service
// granularity each needs the other, so one of them must be wired after
// construction. Both ports are satisfied by paymentSvc directly.
func SetOrderPaymentDeps(orderSvc *order.Service, paymentSvc *payment.Service) {
	orderSvc.SetPaymentDeps(paymentSvc, paymentSvc)
}
