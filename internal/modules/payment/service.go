package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway"
	gatewaymidtrans "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway/midtrans"
	gatewaymock "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway/mock"
	gatewaystripe "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway/stripe"
	paymentjobs "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/jobs"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	jobspg "github.com/residwi/go-api-project-template/internal/modules/payment/jobs/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const jitterDivisor = 2

type Service struct {
	repo    Repository
	tx      database.TxRunner
	gateway Gateway
	jobs    JobRepository
	logger  *slog.Logger

	orders    Orders
	inventory Inventory
	coupon    CouponReleaser

	webhookSecret string

	JobProcessor *paymentjobs.Dispatcher
}

func New(
	repo Repository,
	db database.DB,
	tx database.TxRunner,
	cfg Config,
	logger *slog.Logger,
	orders Orders,
	inventory Inventory,
	coupons CouponReleaser,
) *Service {
	s := &Service{
		repo:          repo,
		tx:            tx,
		gateway:       newGateway(cfg),
		jobs:          jobspg.New(db),
		logger:        logger,
		orders:        orders,
		inventory:     inventory,
		coupon:        coupons,
		webhookSecret: cfg.WebhookSecret,
	}
	s.JobProcessor = paymentjobs.NewDispatcher(s, s, logger)
	return s
}

func newGateway(cfg Config) Gateway {
	switch cfg.Gateway {
	case gatewayStripe:
		return gatewaystripe.New(cfg.GatewayAPIKey, cfg.GatewayTimeout)
	case gatewayMidtrans:
		return gatewaymidtrans.New(cfg.GatewayAPIKey, cfg.GatewayTimeout)
	default:
		return gatewaymock.New(cfg.GatewayURL, cfg.GatewayTimeout)
	}
}

func (s *Service) Charge(ctx context.Context, req ChargeRequest) (ChargeResult, error) {
	existing, err := s.repo.GetActiveByOrderID(ctx, req.OrderID)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return ChargeResult{}, err
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
		if createErr := s.repo.Create(ctx, p); createErr != nil {
			return ChargeResult{}, createErr
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

	resp, err := s.gateway.Charge(ctx, chargeReq)
	if err != nil {
		s.logger.ErrorContext(
			ctx,
			"gateway charge failed",
			slog.String("payment_id", p.ID.String()),
			slog.String("order_id", req.OrderID.String()),
			slog.String("error", err.Error()),
		)
		return ChargeResult{PaymentID: p.ID}, fmt.Errorf("gateway charge: %w", err)
	}

	respJSON, _ := json.Marshal(resp)
	if err := s.repo.UpdateGateway(ctx, p.ID, resp.TransactionID, respJSON); err != nil {
		s.logger.ErrorContext(ctx, "failed to update gateway info", slog.String("error", err.Error()))
	}

	result := ChargeResult{PaymentID: p.ID}

	switch resp.Status {
	case string(domain.StatusSuccess):
		result.Charged = true
		finalizeJob := domain.Job{PaymentID: p.ID, OrderID: req.OrderID, Action: domain.ActionCharge}
		if finalizeErr := s.FinalizeSuccess(
			ctx,
			finalizeJob,
		); finalizeErr != nil &&
			!errors.Is(finalizeErr, apperror.ErrAlreadyFinalized) {
			s.logger.ErrorContext(
				ctx,
				"synchronous charge succeeded but finalization failed, running compensating refund",
				slog.String("payment_id", p.ID.String()),
				slog.String("order_id", req.OrderID.String()),
				slog.String("error", finalizeErr.Error()),
			)
			s.CompensateRefund(ctx, finalizeJob)
		}
	case string(domain.StatusPending):
		if resp.PaymentURL != "" {
			if err := s.repo.UpdatePaymentURL(ctx, p.ID, resp.PaymentURL); err != nil {
				s.logger.ErrorContext(ctx, "failed to update payment url", slog.String("error", err.Error()))
			}
			result.PaymentURL = resp.PaymentURL
		}
	default:
		s.logger.WarnContext(
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

func (s *Service) RunChargeJob(ctx context.Context, job domain.Job) error {
	err := s.orders.MarkPaymentProcessing(ctx, job.OrderID)
	if err != nil {
		s.logger.WarnContext(ctx, "charge job cancelled: order not in expected state",
			slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()))
		job.Status = domain.JobStatusCancelled
		if updateErr := s.jobs.UpdateJob(ctx, &job); updateErr != nil {
			s.logger.ErrorContext(
				ctx,
				"failed to update cancelled job",
				slog.String("error", updateErr.Error()),
			)
		}
		return fmt.Errorf("charge job cancelled: order %s not in expected state", job.OrderID)
	}

	p, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get payment for job", slog.String("error", err.Error()))
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

	resp, gwErr := s.gateway.Charge(ctx, chargeReq)

	job.Attempts++

	if gwErr != nil {
		s.logger.WarnContext(
			ctx,
			"charge failed",
			slog.String("order_id", job.OrderID.String()),
			slog.String("payment_id", job.PaymentID.String()),
			slog.Int("attempt", job.Attempts),
			slog.Int("max_attempts", job.MaxAttempts),
			slog.String("error", gwErr.Error()),
		)

		s.handleChargeFailure(ctx, &job, gwErr.Error())
		return fmt.Errorf("charge failed: %w", gwErr)
	}

	respJSON, _ := json.Marshal(resp)
	if updateErr := s.repo.UpdateGateway(ctx, p.ID, resp.TransactionID, respJSON); updateErr != nil {
		s.logger.ErrorContext(
			ctx,
			"failed to update gateway info",
			slog.String("error", updateErr.Error()),
		)
	}

	switch resp.Status {
	case string(domain.StatusSuccess):
		s.logger.InfoContext(ctx, "charge succeeded",
			slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()),
			slog.String("gateway_txn_id", resp.TransactionID), slog.Int("attempt", job.Attempts))

		if finalizeErr := s.FinalizeSuccess(ctx, job); finalizeErr != nil {
			if errors.Is(finalizeErr, apperror.ErrAlreadyFinalized) {
				s.logger.InfoContext(ctx, "charge job: payment already finalized externally",
					slog.String("order_id", job.OrderID.String()))
				return nil
			}
			s.logger.ErrorContext(ctx, "finalization failed, running compensating flow",
				slog.String("order_id", job.OrderID.String()), slog.String("error", finalizeErr.Error()))
			s.CompensateRefund(ctx, job)
		}
		return nil

	default:
		s.logger.WarnContext(ctx, "charge returned non-success",
			slog.String("order_id", job.OrderID.String()), slog.String("status", resp.Status),
			slog.Int("attempt", job.Attempts))
		s.handleChargeFailure(ctx, &job, fmt.Sprintf("gateway returned status: %s", resp.Status))
		return fmt.Errorf("charge returned non-success status: %s", resp.Status)
	}
}

//nolint:gocognit // single finalize CAS with idempotent already-finalized and late-charge-on-terminal-order branches; funlen counts golines' wrapping, not added logic
func (s *Service) FinalizeSuccess(ctx context.Context, job domain.Job) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		orderSnap, err := s.orders.Snapshot(txCtx, job.OrderID)
		if err != nil {
			return fmt.Errorf("getting order for verification: %w", err)
		}

		p, err := s.repo.GetByID(txCtx, job.PaymentID)
		if err != nil {
			return fmt.Errorf("getting payment for verification: %w", err)
		}

		if !p.Amount.Equal(orderSnap.Total) {
			return apperror.ErrAmountMismatch
		}

		paymentErr := s.repo.MarkPaid(
			txCtx,
			job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			},
		)

		orderErr := s.orders.MarkPaid(txCtx, job.OrderID)

		if paymentErr != nil && orderErr != nil {
			s.logger.InfoContext(txCtx, "job completed: already finalized by external actor (webhook)",
				slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()))
			if markErr := s.jobs.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to mark job completed",
					slog.String("error", markErr.Error()),
				)
			}
			return apperror.ErrAlreadyFinalized
		}

		if orderErr != nil {
			s.logger.ErrorContext(txCtx, "late payment success on terminal order, auto-refund enqueued",
				slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()),
				slog.String("order_status", orderSnap.Status))
			if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, domain.StatusRequiresReview,
				[]domain.Status{domain.StatusSuccess}); statusErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to update payment status to requires_review",
					slog.String("payment_id", job.PaymentID.String()),
					slog.String("error", statusErr.Error()),
				)
			}
			if orderStatusErr := s.orders.MarkFulfillmentFailedAfterCharge(
				txCtx, job.OrderID,
			); orderStatusErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to update order status to fulfillment_failed",
					slog.String("order_id", job.OrderID.String()),
					slog.String("error", orderStatusErr.Error()),
				)
			}

			if createErr := s.enqueueRefund(txCtx, job.PaymentID, job.OrderID); createErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to create refund job",
					slog.String("order_id", job.OrderID.String()),
					slog.String("error", createErr.Error()),
				)
			}
			if markErr := s.jobs.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to mark job completed",
					slog.String("error", markErr.Error()),
				)
			}
			return nil
		}

		items, err := s.orders.ListItemQuantities(txCtx, job.OrderID)
		if err != nil {
			return fmt.Errorf("listing order items: %w", err)
		}

		if err := s.inventory.Deduct(txCtx, items); err != nil {
			return fmt.Errorf("deducting inventory: %w", err)
		}

		if markErr := s.jobs.MarkJobCompleted(txCtx, job.ID); markErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to mark job completed",
				slog.String("error", markErr.Error()),
			)
		}
		return nil
	})
}

func (s *Service) CompensateRefund(ctx context.Context, job domain.Job) {
	txErr := s.tx.Run(ctx, func(txCtx context.Context) error {
		if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing, domain.StatusSuccess}); statusErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update payment status in compensating refund",
				slog.String("payment_id", job.PaymentID.String()),
				slog.String("error", statusErr.Error()),
			)
		}
		if orderErr := s.orders.MarkFulfillmentFailedCompensating(txCtx, job.OrderID); orderErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update order status in compensating refund",
				slog.String("order_id", job.OrderID.String()),
				slog.String("error", orderErr.Error()),
			)
		}

		return s.enqueueRefund(txCtx, job.PaymentID, job.OrderID)
	})
	if txErr != nil {
		s.logger.ErrorContext(ctx, "compensating refund failed",
			slog.String("order_id", job.OrderID.String()), slog.String("error", txErr.Error()))
	}
}

func stockStateFor(deducted bool) inventory.StockState {
	if deducted {
		return inventory.Deducted
	}
	return inventory.Reserved
}

func (s *Service) Refund(ctx context.Context, paymentID uuid.UUID) error {
	p, err := s.repo.GetByID(ctx, paymentID)
	if err != nil {
		return err
	}

	if p.Status != domain.StatusSuccess && p.Status != domain.StatusRequiresReview {
		return fmt.Errorf("%w: payment is not refundable", apperror.ErrBadRequest)
	}

	return s.enqueueRefund(ctx, paymentID, p.OrderID)
}

//nolint:gocognit,funlen // refund retry/backoff bookkeeping plus the gateway-call-then-commit finalization; funlen counts golines' wrapping, not added logic
func (s *Service) RunRefundJob(ctx context.Context, job domain.Job) error {
	p, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		s.logger.ErrorContext(
			ctx,
			"failed to get payment for refund",
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("getting payment for refund: %w", err)
	}

	if p.Status != domain.StatusSuccess && p.Status != domain.StatusRequiresReview {
		s.logger.WarnContext(ctx, "refund job cancelled: payment not refundable",
			slog.String("payment_status", string(p.Status)))
		job.Status = domain.JobStatusCancelled
		if updateErr := s.jobs.UpdateJob(ctx, &job); updateErr != nil {
			s.logger.ErrorContext(
				ctx,
				"failed to update cancelled refund job",
				slog.String("error", updateErr.Error()),
			)
		}
		return fmt.Errorf("refund job cancelled: payment %s not refundable", job.PaymentID)
	}

	s.logger.InfoContext(
		ctx,
		"processing refund",
		slog.String("order_id", job.OrderID.String()),
		slog.String("payment_id", job.PaymentID.String()),
		slog.String("gateway_txn_id", p.GatewayTxnID),
		slog.Int64("amount", p.Amount.Amount),
		slog.String("currency", p.Amount.Currency),
	)

	resp, gwErr := s.gateway.Refund(ctx, gateway.RefundRequest{
		IdempotencyKey: p.ID.String(),
		TransactionID:  p.GatewayTxnID,
		Amount:         p.Amount.Amount,
		Reason:         "auto-refund",
	})

	job.Attempts++

	if gwErr != nil {
		s.logger.ErrorContext(
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
		if updateErr := s.jobs.UpdateJob(ctx, &job); updateErr != nil {
			s.logger.ErrorContext(
				ctx,
				"failed to update refund job after failure",
				slog.String("error", updateErr.Error()),
			)
		}
		return fmt.Errorf("refund failed: %w", gwErr)
	}

	s.logger.InfoContext(ctx, "refund completed",
		slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()),
		slog.String("refund_id", resp.RefundID))

	txErr := s.tx.Run(ctx, func(txCtx context.Context) error {
		orderSnap, snapErr := s.orders.Snapshot(txCtx, job.OrderID)
		if snapErr != nil {
			return fmt.Errorf("getting order for refund: %w", snapErr)
		}

		if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}); statusErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update payment status to refunded",
				slog.String("payment_id", job.PaymentID.String()),
				slog.String("error", statusErr.Error()),
			)
		}
		if orderStatusErr := s.orders.MarkRefunded(txCtx, job.OrderID); orderStatusErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update order status to refunded",
				slog.String("order_id", job.OrderID.String()),
				slog.String("error", orderStatusErr.Error()),
			)
		}

		items, listErr := s.orders.ListItemQuantities(txCtx, job.OrderID)
		if listErr != nil {
			return listErr
		}
		switch {
		case orderSnap.Dispatched:
			s.logger.InfoContext(txCtx, "refund: skipping inventory restock for dispatched order",
				slog.String("order_id", job.OrderID.String()), slog.String("order_status", orderSnap.Status))
		case len(items) > 0 && !orderSnap.StockReversed:
			if restoreErr := s.inventory.Restore(
				txCtx,
				items,
				stockStateFor(orderSnap.StockDeducted),
			); restoreErr != nil {
				s.logger.ErrorContext(txCtx, "failed to restore inventory on refund",
					slog.String("order_id", job.OrderID.String()), slog.String("error", restoreErr.Error()))
			}
		}

		if s.coupon != nil && orderSnap.CouponCode != "" {
			if releaseErr := s.coupon.Release(txCtx, job.OrderID); releaseErr != nil {
				s.logger.WarnContext(
					txCtx,
					"failed to release coupon on refund",
					slog.String("error", releaseErr.Error()),
				)
			}
		}

		if markErr := s.jobs.MarkJobCompleted(txCtx, job.ID); markErr != nil {
			s.logger.ErrorContext(
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

//nolint:gocognit,funlen // resolves the payment then dispatches success/failed/cancelled/expired event branches; funlen counts golines' wrapping, not added logic
func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	if s.webhookSecret != "" && !verifySignature(s.webhookSecret, payload, signature) {
		s.logger.WarnContext(ctx, "webhook: invalid or missing signature")
		return fmt.Errorf("%w: invalid webhook signature", apperror.ErrUnauthorized)
	}

	var body map[string]any
	if err := json.Unmarshal(payload, &body); err != nil {
		s.logger.ErrorContext(ctx, "webhook: invalid payload", slog.String("error", err.Error()))
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
				found, getErr := s.repo.GetByID(ctx, pid)
				if getErr != nil {
					s.logger.ErrorContext(
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
		found, getErr := s.repo.GetByGatewayTxnID(ctx, txnID)
		if getErr != nil {
			if !errors.Is(getErr, apperror.ErrNotFound) {
				s.logger.ErrorContext(
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
		s.logger.ErrorContext(ctx, "webhook: unknown payment_id", slog.String("payload_event", event))
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
		if err := s.FinalizeSuccess(ctx, job); err != nil {
			if errors.Is(err, apperror.ErrAlreadyFinalized) {
				break
			}
			s.logger.ErrorContext(
				ctx,
				"webhook finalization failed, running compensating refund",
				slog.String("payment_id", p.ID.String()),
				slog.String("order_id", p.OrderID.String()),
				slog.String("error", err.Error()),
			)
			s.CompensateRefund(ctx, job)
			return nil
		}
		if err := s.jobs.MarkJobCompletedByPaymentID(ctx, p.ID, domain.ActionCharge); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to mark job completed by payment id",
				slog.String("payment_id", p.ID.String()),
				slog.String("error", err.Error()),
			)
		}

	case "failed", "cancelled", "expired":
		if err := s.repo.UpdateStatus(ctx, p.ID, domain.StatusCancelled,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing}); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to update payment status to cancelled",
				slog.String("payment_id", p.ID.String()),
				slog.String("error", err.Error()),
			)
		}
		if err := s.repo.ClearPaymentURL(ctx, p.ID); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to clear payment url",
				slog.String("payment_id", p.ID.String()),
				slog.String("error", err.Error()),
			)
		}
		if err := s.CancelPendingByOrderID(ctx, p.OrderID); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to cancel jobs",
				slog.String("order_id", p.OrderID.String()),
				slog.String("error", err.Error()),
			)
		}
		if err := s.orders.CancelUnpaid(ctx, p.OrderID); err != nil && !errors.Is(err, apperror.ErrBadRequest) {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to cancel order after payment failure",
				slog.String("order_id", p.OrderID.String()),
				slog.String("error", err.Error()),
			)
		}
		s.logger.InfoContext(ctx, "webhook payment failed",
			slog.String("payment_id", p.ID.String()), slog.String("gateway_event", event))
	}

	return nil
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Payment, int, error) {
	return s.repo.ListAdmin(ctx, params)
}

func (s *Service) handleChargeFailure(ctx context.Context, job *domain.Job, lastError string) {
	job.LastError = lastError

	if err := s.orders.MarkAwaitingPayment(ctx, job.OrderID); err != nil {
		s.logger.ErrorContext(ctx, "failed to CAS order back to awaiting_payment",
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

	if err := s.jobs.UpdateJob(ctx, job); err != nil {
		s.logger.ErrorContext(ctx, "failed to update job after failure",
			slog.String("error", err.Error()))
	}
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
