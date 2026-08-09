// Package changestatus is the admin direct-status-write endpoint. It never
// builds its own from/to list: CanTransition is domain's canonical registry,
// and the actual write goes through transition/'s TransitionPort.
package changestatus

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

type Command struct {
	repo       Repository
	transition TransitionPort
}

func New(repo Repository, transition TransitionPort) *Command {
	return &Command{repo: repo, transition: transition}
}

func (c *Command) Execute(ctx context.Context, orderID uuid.UUID, toStatus domain.Status) error {
	// A bare status write here would mark an order paid without deducting stock,
	// or refunded without restocking it or returning money at the gateway. Any
	// status with those side effects belongs to the flow that owns them.
	// Only the side-effect-free fulfillment markers may be set directly.
	switch toStatus {
	case domain.StatusPaid, domain.StatusPaymentProcessing, domain.StatusCancelled,
		domain.StatusExpired, domain.StatusRefunded, domain.StatusFulfillmentFailed:
		// Reachable from paid, i.e. money captured and stock deducted: a bare write
		// here would strand both. It belongs to the compensating refund flow.
		return fmt.Errorf(
			"%w: status %s is managed by the payment, cancel, or refund flow and cannot be set with a direct status update",
			apperror.ErrBadRequest,
			toStatus,
		)
	case domain.StatusAwaitingPayment, domain.StatusProcessing, domain.StatusShipped, domain.StatusDelivered:
		// None of these reverse inventory or payment, so they may be set directly.
	}

	order, err := c.repo.GetByID(ctx, orderID)
	if err != nil {
		return err
	}

	if !domain.CanTransition(order.Status, toStatus) {
		return fmt.Errorf("%w: cannot transition from %s to %s", apperror.ErrBadRequest, order.Status, toStatus)
	}

	return c.transition.UpdateStatus(ctx, orderID, order.Status, toStatus)
}
