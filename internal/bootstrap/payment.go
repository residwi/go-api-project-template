package bootstrap

import (
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func NewPaymentService(
	repo payment.Repository,
	tx database.TxRunner,
	gw payment.Gateway,
	orderSvc *order.Service,
	inventorySvc *inventory.Service,
	promotionSvc *promotion.Service,
	log *slog.Logger,
) *payment.Service {
	return payment.NewService(
		repo, tx, gw,
		orderSvc,     // satisfies payment.OrderUpdater directly
		orderSvc,     // satisfies payment.OrderGetter directly
		orderSvc,     // satisfies payment.OrderItemsGetter directly
		inventorySvc, // satisfies payment.InventoryDeductor directly
		inventorySvc, // satisfies payment.InventoryRestorer directly
		promotionSvc, // satisfies payment.CouponReleaser directly
		log,
	)
}

func NewOrderHousekeeper(orderSvc *order.Service) payment.OrderHousekeeper {
	return orderSvc
}
