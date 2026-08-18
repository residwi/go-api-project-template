package payment

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
	gatewaymidtrans "github.com/residwi/go-api-project-template/internal/modules/payment/gateway/midtrans"
	gatewaymock "github.com/residwi/go-api-project-template/internal/modules/payment/gateway/mock"
	gatewaystripe "github.com/residwi/go-api-project-template/internal/modules/payment/gateway/stripe"
	"github.com/residwi/go-api-project-template/internal/modules/payment/jobs"
	jobspg "github.com/residwi/go-api-project-template/internal/modules/payment/jobs/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment/usecase/charge"
	chargepg "github.com/residwi/go-api-project-template/internal/modules/payment/usecase/charge/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/payment/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment/usecase/refund"
	refundpg "github.com/residwi/go-api-project-template/internal/modules/payment/usecase/refund/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment/usecase/webhook"
	webhookpg "github.com/residwi/go-api-project-template/internal/modules/payment/usecase/webhook/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Tx     database.TxRunner
	Config Config
	Logger *slog.Logger

	OrderTransition  OrderTransition
	OrderCanceller   OrderCanceller
	OrderReader      OrderReader
	InventoryDeduct  InventoryDeductor
	InventoryRestore InventoryRestorer
	Promotions       CouponPort
}

type OrderTransition interface {
	MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
	MarkPaid(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error
	MarkRefunded(ctx context.Context, orderID uuid.UUID) error
}

type OrderCanceller interface {
	CancelUnpaid(ctx context.Context, orderID uuid.UUID) error
}

type OrderReader interface {
	GetSnapshot(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
	ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error)
}

type InventoryDeductor interface {
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
}

type InventoryRestorer interface {
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

type CouponPort interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}

type Module struct {
	Webhook      *webhook.UseCase
	Query        *query.UseCase
	Refund       *refund.UseCase
	Charge       *charge.UseCase
	Jobs         *jobs.Queue
	JobProcessor *jobs.Dispatcher
}

func New(d Deps) *Module {
	gw := newGateway(d.Config)

	jobsCmd := jobs.New(jobspg.New(d.Pool))

	chargeCmd := charge.New(
		chargepg.New(d.Pool), d.Tx, gw,
		d.OrderTransition, d.OrderReader, d.OrderReader, d.InventoryDeduct, jobsCmd,
		d.Logger,
	)
	refundCmd := refund.New(
		refundpg.New(d.Pool), d.Tx, gw,
		d.OrderTransition, d.OrderReader, d.OrderReader, d.InventoryRestore, d.Promotions, jobsCmd,
		d.Logger,
	)
	dispatcher := jobs.NewDispatcher(chargeCmd, refundCmd, d.Logger)

	webhookCmd := webhook.New(
		webhookpg.New(d.Pool), d.OrderCanceller, chargeCmd, jobsCmd,
		d.Config.WebhookSecret, d.Logger,
	)

	return &Module{
		Webhook:      webhookCmd,
		Query:        query.New(querypg.New(d.Pool)),
		Refund:       refundCmd,
		Charge:       chargeCmd,
		Jobs:         jobsCmd,
		JobProcessor: dispatcher,
	}
}

func newGateway(cfg Config) gateway.Gateway {
	switch cfg.Gateway {
	case gatewayStripe:
		return gatewaystripe.New(cfg.GatewayAPIKey, cfg.GatewayTimeout)
	case gatewayMidtrans:
		return gatewaymidtrans.New(cfg.GatewayAPIKey, cfg.GatewayTimeout)
	default:
		return gatewaymock.New(cfg.GatewayURL, cfg.GatewayTimeout)
	}
}
