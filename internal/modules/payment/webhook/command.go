package webhook

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

type Command struct {
	repo      Repository
	orders    OrderUpdater
	finalizer PaymentFinalizer
	jobs      JobStore
	secret    string
	logger    *slog.Logger
}

func New(
	repo Repository,
	orders OrderUpdater,
	finalizer PaymentFinalizer,
	jobs JobStore,
	secret string,
	log *slog.Logger,
) *Command {
	return &Command{repo: repo, orders: orders, finalizer: finalizer, jobs: jobs, secret: secret, logger: log}
}

//nolint:gocognit,funlen // resolves the payment then dispatches success/failed/cancelled/expired event branches; funlen counts golines' wrapping, not added logic
func (c *Command) Execute(ctx context.Context, payload []byte, signature string) error {
	if c.secret != "" && !verifySignature(c.secret, payload, signature) {
		c.logger.WarnContext(ctx, "webhook: invalid or missing signature")
		return fmt.Errorf("%w: invalid webhook signature", apperror.ErrUnauthorized)
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		c.logger.ErrorContext(ctx, "webhook: invalid payload", slog.String("error", err.Error()))
		return nil
	}

	event, _ := body["event"].(string)
	metadata, _ := body["metadata"].(map[string]any)
	txnID, _ := body["transaction_id"].(string)

	var p *domain.Payment

	if metadata != nil { //nolint:nestif // webhook payload parsing
		if pidStr, ok := metadata["payment_id"].(string); ok {
			pid, parseErr := uuid.Parse(pidStr)
			if parseErr == nil {
				found, getErr := c.repo.GetByID(ctx, pid)
				if getErr != nil {
					c.logger.ErrorContext(
						ctx,
						"webhook: failed to get payment by id",
						slog.String("payment_id", pid.String()),
						slog.String("error", getErr.Error()),
					)
				} else {
					p = found
				}
			}
		}
	}

	if p == nil && txnID != "" {
		found, getErr := c.repo.GetByGatewayTxnID(ctx, txnID)
		if getErr != nil {
			if !errors.Is(getErr, apperror.ErrNotFound) {
				c.logger.ErrorContext(
					ctx,
					"webhook: failed to get payment by gateway txn id",
					slog.String("txn_id", txnID),
					slog.String("error", getErr.Error()),
				)
			}
		} else {
			p = found
		}
	}

	if p == nil {
		c.logger.ErrorContext(ctx, "webhook: unknown payment_id", slog.String("payload_event", event))
		return nil
	}

	if p.Status == domain.StatusSuccess || p.Status == domain.StatusRefunded ||
		p.Status == domain.StatusRequiresReview {
		return nil
	}

	switch event {
	case string(domain.StatusSuccess):
		job := domain.Job{
			PaymentID: p.ID,
			OrderID:   p.OrderID,
			Action:    domain.ActionCharge,
		}
		if err := c.finalizer.FinalizePaymentSuccess(ctx, job); err != nil {
			if errors.Is(err, apperror.ErrAlreadyFinalized) {
				break
			}
			c.logger.ErrorContext(
				ctx,
				"webhook finalization failed, running compensating refund",
				slog.String("payment_id", p.ID.String()),
				slog.String("order_id", p.OrderID.String()),
				slog.String("error", err.Error()),
			)
			c.finalizer.RunCompensatingRefund(ctx, job)
			return nil
		}
		if err := c.jobs.MarkJobCompletedByPaymentID(ctx, p.ID, domain.ActionCharge); err != nil {
			c.logger.ErrorContext(
				ctx,
				"webhook: failed to mark job completed by payment id",
				slog.String("payment_id", p.ID.String()),
				slog.String("error", err.Error()),
			)
		}

	case "failed", "cancelled", "expired":
		if err := c.repo.UpdateStatus(ctx, p.ID, domain.StatusCancelled,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing}); err != nil {
			c.logger.ErrorContext(
				ctx,
				"webhook: failed to update payment status to cancelled",
				slog.String("payment_id", p.ID.String()),
				slog.String("error", err.Error()),
			)
		}
		if err := c.repo.ClearPaymentURL(ctx, p.ID); err != nil {
			c.logger.ErrorContext(
				ctx,
				"webhook: failed to clear payment url",
				slog.String("payment_id", p.ID.String()),
				slog.String("error", err.Error()),
			)
		}
		if err := c.jobs.CancelPendingByOrderID(ctx, p.OrderID); err != nil {
			c.logger.ErrorContext(
				ctx,
				"webhook: failed to cancel jobs",
				slog.String("order_id", p.OrderID.String()),
				slog.String("error", err.Error()),
			)
		}
		if err := c.orders.CancelUnpaid(ctx, p.OrderID); err != nil && !errors.Is(err, apperror.ErrBadRequest) {
			c.logger.ErrorContext(
				ctx,
				"webhook: failed to cancel order after payment failure",
				slog.String("order_id", p.OrderID.String()),
				slog.String("error", err.Error()),
			)
		}
		c.logger.InfoContext(ctx, "webhook payment failed",
			slog.String("payment_id", p.ID.String()), slog.String("gateway_event", event))
	}

	return nil
}

func verifySignature(secret string, body []byte, provided string) bool {
	if provided == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(provided))
}
