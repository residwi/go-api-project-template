package charge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const jitterDivisor = 2

type UseCase struct {
	repo       Repository
	tx         database.TxRunner
	gateway    Gateway
	orders     OrderUpdater
	orderGet   OrderGetter
	orderItems OrderItemsGetter
	inventory  InventoryDeductor
	jobs       JobStore
	logger     *slog.Logger
}

func New(
	repo Repository,
	tx database.TxRunner,
	gw Gateway,
	orders OrderUpdater,
	orderGet OrderGetter,
	orderItems OrderItemsGetter,
	inventory InventoryDeductor,
	jobs JobStore,
	log *slog.Logger,
) *UseCase {
	return &UseCase{
		repo:       repo,
		tx:         tx,
		gateway:    gw,
		orders:     orders,
		orderGet:   orderGet,
		orderItems: orderItems,
		inventory:  inventory,
		jobs:       jobs,
		logger:     log,
	}
}

func (c *UseCase) InitiatePayment(ctx context.Context, req contract.ChargeRequest) (contract.ChargeResult, error) {
	existing, err := c.repo.GetActiveByOrderID(ctx, req.OrderID)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return contract.ChargeResult{}, err
	}

	var p *domain.Payment
	if !errors.Is(err, apperror.ErrNotFound) {
		p = existing
	} else {
		p = &domain.Payment{
			OrderID:         req.OrderID,
			Amount:          req.Amount,
			Status:          domain.StatusPending,
			PaymentMethodID: req.PaymentMethodID,
		}
		if createErr := c.repo.Create(ctx, p); createErr != nil {
			return contract.ChargeResult{}, createErr
		}
	}

	chargeReq := gateway.ChargeRequest{
		IdempotencyKey:  p.ID.String(),
		OrderID:         req.OrderID.String(),
		Amount:          req.Amount.Amount,
		Currency:        req.Amount.Currency,
		PaymentMethodID: req.PaymentMethodID,
		Metadata:        map[string]string{"payment_id": p.ID.String()},
	}

	resp, err := c.gateway.Charge(ctx, chargeReq)
	if err != nil {
		c.logger.ErrorContext(
			ctx,
			"gateway charge failed",
			slog.String("payment_id", p.ID.String()),
			slog.String("order_id", req.OrderID.String()),
			slog.String("error", err.Error()),
		)
		return contract.ChargeResult{PaymentID: p.ID}, fmt.Errorf("gateway charge: %w", err)
	}

	respJSON, _ := json.Marshal(resp)
	if err := c.repo.UpdateGateway(ctx, p.ID, resp.TransactionID, respJSON); err != nil {
		c.logger.ErrorContext(ctx, "failed to update gateway info", slog.String("error", err.Error()))
	}

	result := contract.ChargeResult{PaymentID: p.ID}

	switch resp.Status {
	case string(domain.StatusSuccess):
		result.Charged = true
		finalizeJob := domain.Job{PaymentID: p.ID, OrderID: req.OrderID, Action: domain.ActionCharge}
		if finalizeErr := c.FinalizePaymentSuccess(
			ctx,
			finalizeJob,
		); finalizeErr != nil &&
			!errors.Is(finalizeErr, apperror.ErrAlreadyFinalized) {
			c.logger.ErrorContext(
				ctx,
				"synchronous charge succeeded but finalization failed, running compensating refund",
				slog.String("payment_id", p.ID.String()),
				slog.String("order_id", req.OrderID.String()),
				slog.String("error", finalizeErr.Error()),
			)
			c.RunCompensatingRefund(ctx, finalizeJob)
		}
	case string(domain.StatusPending):
		if resp.PaymentURL != "" {
			if err := c.repo.UpdatePaymentURL(ctx, p.ID, resp.PaymentURL); err != nil {
				c.logger.ErrorContext(ctx, "failed to update payment url", slog.String("error", err.Error()))
			}
			result.PaymentURL = resp.PaymentURL
		}
	default:
		c.logger.WarnContext(
			ctx,
			"gateway declined charge synchronously",
			slog.String("payment_id", p.ID.String()),
			slog.String("order_id", req.OrderID.String()),
			slog.String("gateway_status", resp.Status),
		)
		return result, fmt.Errorf("%w: payment was declined", apperror.ErrBadRequest)
	}

	return result, nil
}

func (c *UseCase) ProcessCharge(ctx context.Context, job domain.Job) error {
	err := c.orders.MarkPaymentProcessing(ctx, job.OrderID)
	if err != nil {
		c.logger.WarnContext(ctx, "charge job cancelled: order not in expected state",
			slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()))
		job.Status = domain.JobStatusCancelled
		if updateErr := c.jobs.UpdateJob(ctx, &job); updateErr != nil {
			c.logger.ErrorContext(
				ctx,
				"failed to update cancelled job",
				slog.String("error", updateErr.Error()),
			)
		}
		return fmt.Errorf("charge job cancelled: order %s not in expected state", job.OrderID)
	}

	p, err := c.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		c.logger.ErrorContext(ctx, "failed to get payment for job", slog.String("error", err.Error()))
		return fmt.Errorf("getting payment for job: %w", err)
	}

	chargeReq := gateway.ChargeRequest{
		IdempotencyKey:  p.ID.String(),
		OrderID:         job.OrderID.String(),
		Amount:          p.Amount.Amount,
		Currency:        p.Amount.Currency,
		PaymentMethodID: p.PaymentMethodID,
		Metadata:        map[string]string{"payment_id": p.ID.String()},
	}

	resp, gwErr := c.gateway.Charge(ctx, chargeReq)

	job.Attempts++

	if gwErr != nil {
		c.logger.WarnContext(
			ctx,
			"charge failed",
			slog.String("order_id", job.OrderID.String()),
			slog.String("payment_id", job.PaymentID.String()),
			slog.Int("attempt", job.Attempts),
			slog.Int("max_attempts", job.MaxAttempts),
			slog.String("error", gwErr.Error()),
		)

		c.handleChargeFailure(ctx, &job, gwErr.Error())
		return fmt.Errorf("charge failed: %w", gwErr)
	}

	respJSON, _ := json.Marshal(resp)
	if updateErr := c.repo.UpdateGateway(ctx, p.ID, resp.TransactionID, respJSON); updateErr != nil {
		c.logger.ErrorContext(
			ctx,
			"failed to update gateway info",
			slog.String("error", updateErr.Error()),
		)
	}

	switch resp.Status {
	case string(domain.StatusSuccess):
		c.logger.InfoContext(ctx, "charge succeeded",
			slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()),
			slog.String("gateway_txn_id", resp.TransactionID), slog.Int("attempt", job.Attempts))

		if finalizeErr := c.FinalizePaymentSuccess(ctx, job); finalizeErr != nil {
			if errors.Is(finalizeErr, apperror.ErrAlreadyFinalized) {
				c.logger.InfoContext(ctx, "charge job: payment already finalized externally",
					slog.String("order_id", job.OrderID.String()))
				return nil
			}
			c.logger.ErrorContext(ctx, "finalization failed, running compensating flow",
				slog.String("order_id", job.OrderID.String()), slog.String("error", finalizeErr.Error()))
			c.RunCompensatingRefund(ctx, job)
		}
		return nil

	default:
		c.logger.WarnContext(ctx, "charge returned non-success",
			slog.String("order_id", job.OrderID.String()), slog.String("status", resp.Status),
			slog.Int("attempt", job.Attempts))
		c.handleChargeFailure(ctx, &job, fmt.Sprintf("gateway returned status: %s", resp.Status))
		return fmt.Errorf("charge returned non-success status: %s", resp.Status)
	}
}

//nolint:gocognit // single finalize CAS with idempotent already-finalized and late-charge-on-terminal-order branches; funlen counts golines' wrapping, not added logic
func (c *UseCase) FinalizePaymentSuccess(ctx context.Context, job domain.Job) error {
	return c.tx.Run(ctx, func(txCtx context.Context) error {
		orderSnap, err := c.orderGet.GetSnapshot(txCtx, job.OrderID)
		if err != nil {
			return fmt.Errorf("getting order for verification: %w", err)
		}

		p, err := c.repo.GetByID(txCtx, job.PaymentID)
		if err != nil {
			return fmt.Errorf("getting payment for verification: %w", err)
		}

		if !p.Amount.Equal(orderSnap.Total) {
			return apperror.ErrAmountMismatch
		}

		paymentErr := c.repo.MarkPaid(
			txCtx,
			job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			},
		)

		orderErr := c.orders.MarkPaid(txCtx, job.OrderID)

		if paymentErr != nil && orderErr != nil {
			c.logger.InfoContext(txCtx, "job completed: already finalized by external actor (webhook)",
				slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()))
			if markErr := c.jobs.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				c.logger.ErrorContext(
					txCtx,
					"failed to mark job completed",
					slog.String("error", markErr.Error()),
				)
			}
			return apperror.ErrAlreadyFinalized
		}

		if orderErr != nil {
			c.logger.ErrorContext(txCtx, "late payment success on terminal order, auto-refund enqueued",
				slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()),
				slog.String("order_status", orderSnap.Status))
			if statusErr := c.repo.UpdateStatus(txCtx, job.PaymentID, domain.StatusRequiresReview,
				[]domain.Status{domain.StatusSuccess}); statusErr != nil {
				c.logger.ErrorContext(
					txCtx,
					"failed to update payment status to requires_review",
					slog.String("payment_id", job.PaymentID.String()),
					slog.String("error", statusErr.Error()),
				)
			}
			if orderStatusErr := c.orders.MarkFulfillmentFailedAfterCharge(txCtx, job.OrderID); orderStatusErr != nil {
				c.logger.ErrorContext(
					txCtx,
					"failed to update order status to fulfillment_failed",
					slog.String("order_id", job.OrderID.String()),
					slog.String("error", orderStatusErr.Error()),
				)
			}

			if createErr := c.jobs.EnqueueRefund(txCtx, job.PaymentID, job.OrderID); createErr != nil {
				c.logger.ErrorContext(
					txCtx,
					"failed to create refund job",
					slog.String("order_id", job.OrderID.String()),
					slog.String("error", createErr.Error()),
				)
			}
			if markErr := c.jobs.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				c.logger.ErrorContext(
					txCtx,
					"failed to mark job completed",
					slog.String("error", markErr.Error()),
				)
			}
			return nil
		}

		items, err := c.orderItems.ListItemQuantities(txCtx, job.OrderID)
		if err != nil {
			return fmt.Errorf("listing order items: %w", err)
		}

		if err := c.inventory.DeductBatch(txCtx, items); err != nil {
			return fmt.Errorf("deducting inventory: %w", err)
		}

		if markErr := c.jobs.MarkJobCompleted(txCtx, job.ID); markErr != nil {
			c.logger.ErrorContext(
				txCtx,
				"failed to mark job completed",
				slog.String("error", markErr.Error()),
			)
		}
		return nil
	})
}

func (c *UseCase) RunCompensatingRefund(ctx context.Context, job domain.Job) {
	txErr := c.tx.Run(ctx, func(txCtx context.Context) error {
		if statusErr := c.repo.UpdateStatus(txCtx, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing, domain.StatusSuccess}); statusErr != nil {
			c.logger.ErrorContext(
				txCtx,
				"failed to update payment status in compensating refund",
				slog.String("payment_id", job.PaymentID.String()),
				slog.String("error", statusErr.Error()),
			)
		}
		if orderErr := c.orders.MarkFulfillmentFailedCompensating(txCtx, job.OrderID); orderErr != nil {
			c.logger.ErrorContext(
				txCtx,
				"failed to update order status in compensating refund",
				slog.String("order_id", job.OrderID.String()),
				slog.String("error", orderErr.Error()),
			)
		}

		return c.jobs.EnqueueRefund(txCtx, job.PaymentID, job.OrderID)
	})
	if txErr != nil {
		c.logger.ErrorContext(ctx, "compensating refund failed",
			slog.String("order_id", job.OrderID.String()), slog.String("error", txErr.Error()))
	}
}

func (c *UseCase) handleChargeFailure(ctx context.Context, job *domain.Job, lastError string) {
	job.LastError = lastError

	if err := c.orders.MarkAwaitingPayment(ctx, job.OrderID); err != nil {
		c.logger.ErrorContext(ctx, "failed to CAS order back to awaiting_payment",
			slog.String("order_id", job.OrderID.String()), slog.String("error", err.Error()))
	}

	if job.Attempts >= job.MaxAttempts {
		job.Status = domain.JobStatusFailed
		job.LockedUntil = nil
	} else {
		job.Status = domain.JobStatusPending
		job.LockedUntil = nil
		backoff := time.Duration(1<<min(max(job.Attempts, 0), 30)) * time.Second
		jitter := time.Duration(
			rand.N(int64(backoff / jitterDivisor)), //nolint:gosec // jitter doesn't need crypto randomness
		)
		nextRetry := time.Now().Add(backoff + jitter)
		job.NextRetryAt = nextRetry
	}

	if err := c.jobs.UpdateJob(ctx, job); err != nil {
		c.logger.ErrorContext(ctx, "failed to update job after failure",
			slog.String("error", err.Error()))
	}
}
