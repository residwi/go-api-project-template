// Package transition applies order's state machine. Every allowed-from set is
// declared once, in domain/transition.go; this package only ever hands a named
// domain.Transition to the guarded compare-and-set, never a from/to list of its
// own.
package transition

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

// Applier is consumed by other modules and slices through narrow ports they
// declare themselves (place's TransitionApplier, changestatus's TransitionPort,
// ...) and by payment/shipping directly by name-match on the Mark* methods, so
// no adapter stands between them.
type Applier struct {
	repo Repository
}

func New(repo Repository) *Applier {
	return &Applier{repo: repo}
}

// Apply is the single entry point for a status change: a compare-and-set that
// returns apperror.ErrConflict when the current status is not in t.From.
// Callers name a transition from domain/transition.go, never an ad-hoc status
// list.
func (a *Applier) Apply(ctx context.Context, orderID uuid.UUID, t domain.Transition) error {
	return a.repo.Apply(ctx, orderID, t)
}

// UpdateStatus backs changestatus's admin flow: the same guarded write
// discipline as Apply, but for a dynamic to-status with no stock flags.
func (a *Applier) UpdateStatus(ctx context.Context, orderID uuid.UUID, from, to domain.Status) error {
	return a.repo.UpdateStatus(ctx, orderID, from, to)
}

// The Mark* methods below are named for the capability their callers ask for,
// so payment.OrderUpdater and shipping's OrderPort are satisfied without an
// adapter. Each maps to exactly one named Transition, so the allowed-from set
// stays declared once, in domain/transition.go.

func (a *Applier) MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.PaymentProcessingTransition)
}

func (a *Applier) MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.AwaitingPaymentTransition)
}

func (a *Applier) MarkPaid(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.PaidTransition)
}

func (a *Applier) MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.FulfillmentFailedAfterChargeTransition)
}

func (a *Applier) MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.FulfillmentFailedCompensatingTransition)
}

func (a *Applier) MarkRefunded(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.RefundTransition)
}

func (a *Applier) MarkShipped(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.ShippedTransition)
}

func (a *Applier) MarkDelivered(ctx context.Context, orderID uuid.UUID) error {
	return a.Apply(ctx, orderID, domain.DeliveredTransition)
}
