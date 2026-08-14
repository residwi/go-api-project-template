package refund

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const jitterDivisor = 2

func stockStateFor(deducted bool) inventorycontract.StockState {
	if deducted {
		return inventorycontract.Deducted
	}
	return inventorycontract.Reserved
}

type Command struct {
	repo              Repository
	tx                database.TxRunner
	gateway           Gateway
	orders            OrderUpdater
	orderGet          OrderGetter
	orderItems        OrderItemsGetter
	inventoryRestorer InventoryRestorer
	couponReleaser    CouponReleaser
	jobs              JobStore
	logger            *slog.Logger
}

func New(
	repo Repository,
	tx database.TxRunner,
	gw Gateway,
	orders OrderUpdater,
	orderGet OrderGetter,
	orderItems OrderItemsGetter,
	inventoryRestorer InventoryRestorer,
	couponReleaser CouponReleaser,
	jobs JobStore,
	log *slog.Logger,
) *Command {
	return &Command{
		repo:              repo,
		tx:                tx,
		gateway:           gw,
		orders:            orders,
		orderGet:          orderGet,
		orderItems:        orderItems,
		inventoryRestorer: inventoryRestorer,
		couponReleaser:    couponReleaser,
		jobs:              jobs,
		logger:            log,
	}
}

func (c *Command) Execute(ctx context.Context, paymentID uuid.UUID) error {
	p, err := c.repo.GetByID(ctx, paymentID)
	if err != nil {
		return err
	}

	if p.Status != domain.StatusSuccess && p.Status != domain.StatusRequiresReview {
		return fmt.Errorf("%w: payment is not refundable", apperror.ErrBadRequest)
	}

	return c.jobs.EnqueueRefund(ctx, paymentID, p.OrderID)
}

//nolint:gocognit,funlen // refund retry/backoff bookkeeping plus the gateway-call-then-commit finalization; funlen counts golines' wrapping, not added logic
func (c *Command) ProcessRefund(ctx context.Context, job domain.Job) error {
	p, err := c.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		c.logger.ErrorContext(
			ctx,
			"failed to get payment for refund",
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("getting payment for refund: %w", err)
	}

	if p.Status != domain.StatusSuccess && p.Status != domain.StatusRequiresReview {
		c.logger.WarnContext(ctx, "refund job cancelled: payment not refundable",
			slog.String("payment_status", string(p.Status)))
		job.Status = domain.JobStatusCancelled
		if updateErr := c.jobs.UpdateJob(ctx, &job); updateErr != nil {
			c.logger.ErrorContext(
				ctx,
				"failed to update cancelled refund job",
				slog.String("error", updateErr.Error()),
			)
		}
		return fmt.Errorf("refund job cancelled: payment %s not refundable", job.PaymentID)
	}

	c.logger.InfoContext(
		ctx,
		"processing refund",
		slog.String("order_id", job.OrderID.String()),
		slog.String("payment_id", job.PaymentID.String()),
		slog.String("gateway_txn_id", p.GatewayTxnID),
		slog.Int64("amount", p.Amount.Amount),
		slog.String("currency", p.Amount.Currency),
	)

	resp, gwErr := c.gateway.Refund(ctx, gateway.RefundRequest{
		IdempotencyKey: p.ID.String(),
		TransactionID:  p.GatewayTxnID,
		Amount:         p.Amount.Amount,
		Reason:         "auto-refund",
	})

	job.Attempts++

	if gwErr != nil {
		c.logger.ErrorContext(
			ctx,
			"refund failed",
			slog.String("order_id", job.OrderID.String()),
			slog.Int("attempt", job.Attempts),
			slog.String("error", gwErr.Error()),
		)
		job.LastError = gwErr.Error()

		if job.Attempts >= job.MaxAttempts {
			job.Status = domain.JobStatusFailed
		} else {
			job.Status = domain.JobStatusPending
			backoff := time.Duration(1<<min(max(job.Attempts, 0), 30)) * time.Second
			jitter := time.Duration(
				rand.N(int64(backoff / jitterDivisor)), //nolint:gosec // jitter doesn't need crypto randomness
			)
			job.NextRetryAt = time.Now().Add(backoff + jitter)
		}
		job.LockedUntil = nil
		if updateErr := c.jobs.UpdateJob(ctx, &job); updateErr != nil {
			c.logger.ErrorContext(
				ctx,
				"failed to update refund job after failure",
				slog.String("error", updateErr.Error()),
			)
		}
		return fmt.Errorf("refund failed: %w", gwErr)
	}

	c.logger.InfoContext(ctx, "refund completed",
		slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()),
		slog.String("refund_id", resp.RefundID))

	txErr := c.tx.Run(ctx, func(txCtx context.Context) error {
		orderSnap, snapErr := c.orderGet.GetSnapshot(txCtx, job.OrderID)
		if snapErr != nil {
			return fmt.Errorf("getting order for refund: %w", snapErr)
		}

		if statusErr := c.repo.UpdateStatus(txCtx, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}); statusErr != nil {
			c.logger.ErrorContext(
				txCtx,
				"failed to update payment status to refunded",
				slog.String("payment_id", job.PaymentID.String()),
				slog.String("error", statusErr.Error()),
			)
		}
		if orderStatusErr := c.orders.MarkRefunded(txCtx, job.OrderID); orderStatusErr != nil {
			c.logger.ErrorContext(
				txCtx,
				"failed to update order status to refunded",
				slog.String("order_id", job.OrderID.String()),
				slog.String("error", orderStatusErr.Error()),
			)
		}

		items, listErr := c.orderItems.ListItemQuantities(txCtx, job.OrderID)
		if listErr != nil {
			return listErr
		}
		switch {
		case orderSnap.Dispatched:
			c.logger.InfoContext(txCtx, "refund: skipping inventory restock for dispatched order",
				slog.String("order_id", job.OrderID.String()), slog.String("order_status", orderSnap.Status))
		case len(items) > 0 && !orderSnap.StockReversed:
			if restoreErr := c.inventoryRestorer.Restore(
				txCtx,
				items,
				stockStateFor(orderSnap.StockDeducted),
			); restoreErr != nil {
				c.logger.ErrorContext(txCtx, "failed to restore inventory on refund",
					slog.String("order_id", job.OrderID.String()), slog.String("error", restoreErr.Error()))
			}
		}

		if c.couponReleaser != nil && orderSnap.CouponCode != "" {
			if releaseErr := c.couponReleaser.Release(txCtx, job.OrderID); releaseErr != nil {
				c.logger.WarnContext(
					txCtx,
					"failed to release coupon on refund",
					slog.String("error", releaseErr.Error()),
				)
			}
		}

		if markErr := c.jobs.MarkJobCompleted(txCtx, job.ID); markErr != nil {
			c.logger.ErrorContext(
				txCtx,
				"failed to mark refund job completed",
				slog.String("error", markErr.Error()),
			)
		}
		return nil
	})

	if txErr != nil {
		return fmt.Errorf("refund finalization failed: %w", txErr)
	}
	return nil
}
