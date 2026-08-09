// Package payment composes payment's slices. It imports no transport
// package, so cmd/worker can construct this module without linking HTTP.
package payment

import (
	"context"
	"log/slog"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/charge"
	chargepg "github.com/residwi/go-api-project-template/internal/modules/payment/charge/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
	gatewaymidtrans "github.com/residwi/go-api-project-template/internal/modules/payment/gateway/midtrans"
	gatewaymock "github.com/residwi/go-api-project-template/internal/modules/payment/gateway/mock"
	gatewaystripe "github.com/residwi/go-api-project-template/internal/modules/payment/gateway/stripe"
	"github.com/residwi/go-api-project-template/internal/modules/payment/jobs"
	jobspg "github.com/residwi/go-api-project-template/internal/modules/payment/jobs/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/payment/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment/refund"
	refundpg "github.com/residwi/go-api-project-template/internal/modules/payment/refund/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/payment/webhook"
	webhookpg "github.com/residwi/go-api-project-template/internal/modules/payment/webhook/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool   *pgxpool.Pool
	Tx     database.TxRunner
	Config Config
	Logger *slog.Logger

	// Cross-module ports, satisfied by name-match.
	OrderTransition OrderTransition
	OrderCanceller  OrderCanceller
	OrderReader     OrderReader
	Inventory       InventoryPort
	Promotions      CouponPort
}

// OrderTransition is the union of order.Mark* methods charge and refund drive
// between them: order.Module's Transition delegators satisfy it directly.
// Splitting this into two separate per-slice interfaces (charge.OrderUpdater,
// refund.OrderUpdater) is what keeps each slice's own port exactly as wide as
// it needs -- this bundle exists only because Deps hands one value to both.
//
// CancelUnpaid is deliberately not part of this bundle: it is webhook's alone,
// and unlike a bare Mark* it runs order/cancel's full reversal (stock, coupon,
// guarded status CAS), which needs its own inventory and coupon deps -- a
// bundle of transition-only methods could never satisfy it. See OrderCanceller.
type OrderTransition interface {
	MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
	MarkPaid(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error
	MarkRefunded(ctx context.Context, orderID uuid.UUID) error
}

// OrderCanceller is webhook's alone: order.Cancel owns the entire reversal
// behind this one call. Bootstrap builds a second, throwaway order/cancel
// command for this -- same reasoning as OrderTransition/OrderReader above --
// because CancelUnpaid never reads the paymentCancel dependency the "real"
// order/cancel that order.Module owns is built with, only the user-triggered
// Execute path does.
type OrderCanceller interface {
	CancelUnpaid(ctx context.Context, orderID uuid.UUID) error
}

// OrderReader is what charge, refund and webhook need to read from order.
type OrderReader interface {
	GetSnapshot(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
	ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error)
}

// InventoryPort is the union of what charge and refund need from inventory.
type InventoryPort interface {
	DeductBatch(ctx context.Context, items map[uuid.UUID]int) error
	Restore(ctx context.Context, items map[uuid.UUID]int, prior inventorycontract.StockState) error
}

// CouponPort is what refund needs from promotion to release a reservation.
type CouponPort interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}

// Module is Webhook, Query, Refund, Charge and Jobs. Charge satisfies order's
// PaymentInitiator; Jobs satisfies order/cancel's PaymentJobCanceller.
type Module struct {
	Webhook *webhook.Command
	Query   *query.Reader
	Refund  *refund.Command
	Charge  *charge.Command
	Jobs    *jobs.Command
}

func New(d Deps) *Module {
	gw := newGateway(d.Config)

	// jobs needs nothing but the pool: charge and refund each need jobs back
	// (to enqueue a follow-up job or settle their own), so jobs is built first
	// and SetProcessors wires the other two in once they exist -- payment's own
	// internal cycle, contained entirely inside this function.
	jobsCmd := jobs.New(jobspg.New(d.Pool), d.Logger)

	chargeCmd := charge.New(
		chargepg.New(d.Pool), d.Tx, gw,
		d.OrderTransition, d.OrderReader, d.OrderReader, d.Inventory, jobsCmd,
		d.Logger,
	)
	refundCmd := refund.New(
		refundpg.New(d.Pool), d.Tx, gw,
		d.OrderTransition, d.OrderReader, d.OrderReader, d.Inventory, d.Promotions, jobsCmd,
		d.Logger,
	)
	jobsCmd.SetProcessors(chargeCmd, refundCmd)

	webhookCmd := webhook.New(
		webhookpg.New(d.Pool), d.OrderCanceller, chargeCmd, jobsCmd,
		d.Config.WebhookSecret, d.Logger,
	)

	return &Module{
		Webhook: webhookCmd,
		Query:   query.New(querypg.New(d.Pool)),
		Refund:  refundCmd,
		Charge:  chargeCmd,
		Jobs:    jobsCmd,
	}
}

// newGateway picks one Gateway implementation from Config.Gateway. gateway/
// is an adapter family, not a slice: charge and refund both depend on the
// same two-method Gateway interface, so module.go is the one place that
// chooses which real implementation backs it.
func newGateway(cfg Config) gateway.Gateway {
	switch cfg.Gateway {
	case "stripe":
		return gatewaystripe.New(cfg.GatewayAPIKey, cfg.GatewayTimeout)
	case "midtrans":
		return gatewaymidtrans.New(cfg.GatewayAPIKey, cfg.GatewayTimeout)
	default:
		return gatewaymock.New(cfg.GatewayURL, cfg.GatewayTimeout)
	}
}
