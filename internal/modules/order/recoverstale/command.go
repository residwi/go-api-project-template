// Package recoverstale un-strands orders left in payment_processing by a
// worker that died mid-charge, handing them back to the retry/expiry path.
package recoverstale

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
)

const housekeepingBatchLimit = 20

type Command struct {
	repo       Repository
	transition TransitionApplier
	logger     *slog.Logger
}

func New(repo Repository, transition TransitionApplier, log *slog.Logger) *Command {
	return &Command{repo: repo, transition: transition, logger: log}
}

// Sweep is the worker's per-tick hook. The CAS only matches payment_processing,
// so a concurrent recovery no-ops.
func (c *Command) Sweep(ctx context.Context) error {
	orders, err := c.repo.GetStaleProcessingOrders(ctx, contract.StaleProcessingThreshold, housekeepingBatchLimit)
	if err != nil {
		return fmt.Errorf("getting stale processing orders: %w", err)
	}
	for _, o := range orders {
		if err := c.transition.Apply(ctx, o.ID, domain.AwaitingPaymentTransition); err != nil {
			if errors.Is(err, apperror.ErrConflict) {
				continue // already moved on by another worker
			}
			c.logger.ErrorContext(
				ctx,
				"failed to recover stale processing order",
				slog.String("order_id", o.ID.String()),
				slog.String("error", err.Error()),
			)
		}
	}
	return nil
}
