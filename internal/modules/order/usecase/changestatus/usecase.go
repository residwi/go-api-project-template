package changestatus

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

type UseCase struct {
	repo       Repository
	transition TransitionPort
}

func New(repo Repository, transition TransitionPort) *UseCase {
	return &UseCase{repo: repo, transition: transition}
}

func (c *UseCase) Execute(ctx context.Context, orderID uuid.UUID, toStatus domain.Status) error {
	switch toStatus {
	case domain.StatusPaid, domain.StatusPaymentProcessing, domain.StatusCancelled,
		domain.StatusExpired, domain.StatusRefunded, domain.StatusFulfillmentFailed:
		return fmt.Errorf(
			"%w: status %s is managed by the payment, cancel, or refund flow and cannot be set with a direct status update",
			apperror.ErrBadRequest,
			toStatus,
		)
	case domain.StatusAwaitingPayment, domain.StatusProcessing, domain.StatusShipped, domain.StatusDelivered:
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
