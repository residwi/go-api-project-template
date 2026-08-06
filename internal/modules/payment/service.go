package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/logger"
)

const jitterDivisor = 2

func toInventoryChanges(items []OrderItemDTO) []InventoryChange {
	changes := make([]InventoryChange, len(items))
	for i, it := range items {
		changes[i] = InventoryChange(it)
	}
	return changes
}

type Service struct {
	repo              Repository
	tx                database.TxRunner
	gateway           Gateway
	orders            OrderUpdater
	orderGet          OrderGetter
	orderItems        OrderItemsGetter
	inventory         InventoryDeductor
	inventoryRestorer InventoryRestorer
	couponReleaser    CouponReleaser
	logger            *slog.Logger
}

func NewService(
	repo Repository,
	tx database.TxRunner,
	gw Gateway,
	orders OrderUpdater,
	orderGet OrderGetter,
	orderItems OrderItemsGetter,
	inventory InventoryDeductor,
	inventoryRestorer InventoryRestorer,
	couponReleaser CouponReleaser,
	log *slog.Logger,
) *Service {
	return &Service{
		repo:              repo,
		tx:                tx,
		gateway:           gw,
		orders:            orders,
		orderGet:          orderGet,
		orderItems:        orderItems,
		inventory:         inventory,
		inventoryRestorer: inventoryRestorer,
		couponReleaser:    couponReleaser,
		logger:            log,
	}
}

type InitiatePaymentParams struct {
	OrderID         uuid.UUID
	Amount          money.Money
	PaymentMethodID string
}

type InitiatePaymentResult struct {
	PaymentID  uuid.UUID
	PaymentURL string
	Charged    bool
}

func (s *Service) InitiatePayment(ctx context.Context, params InitiatePaymentParams) (InitiatePaymentResult, error) {
	existing, err := s.repo.GetActiveByOrderID(ctx, params.OrderID)
	if err != nil && !errors.Is(err, apperror.ErrNotFound) {
		return InitiatePaymentResult{}, err
	}

	var p *Payment
	if !errors.Is(err, apperror.ErrNotFound) {
		p = existing
	} else {
		p = &Payment{
			OrderID:         params.OrderID,
			Amount:          params.Amount,
			Status:          StatusPending,
			PaymentMethodID: params.PaymentMethodID,
		}
		if createErr := s.repo.Create(ctx, p); createErr != nil {
			return InitiatePaymentResult{}, createErr
		}
	}

	// ChargeRequest is the gateway's wire contract: this is the seam, not a leak.
	chargeReq := ChargeRequest{
		IdempotencyKey:  p.ID.String(),
		OrderID:         params.OrderID.String(),
		Amount:          params.Amount.Amount,
		Currency:        params.Amount.Currency,
		PaymentMethodID: params.PaymentMethodID,
		Metadata:        map[string]string{"payment_id": p.ID.String()},
	}

	resp, err := s.gateway.Charge(ctx, chargeReq)
	if err != nil {
		s.logger.ErrorContext(
			ctx,
			"gateway charge failed",
			slog.String(
				"payment_id",
				p.ID.String(),
			),
			slog.String("order_id", params.OrderID.String()),
			slog.String("error", err.Error()),
		)
		return InitiatePaymentResult{PaymentID: p.ID}, fmt.Errorf("gateway charge: %w", err)
	}

	respJSON, _ := json.Marshal(resp)
	if err := s.repo.UpdateGateway(ctx, p.ID, resp.TransactionID, respJSON); err != nil {
		s.logger.ErrorContext(ctx, "failed to update gateway info", slog.String("error", err.Error()))
	}

	result := InitiatePaymentResult{PaymentID: p.ID}

	switch resp.Status {
	case string(StatusSuccess):
		result.Charged = true
		// Funds are already captured, so finalize now rather than wait for a webhook
		// or job that never comes.
		finalizeJob := Job{PaymentID: p.ID, OrderID: params.OrderID, Action: ActionCharge}
		if finalizeErr := s.FinalizePaymentSuccess(
			ctx,
			finalizeJob,
		); finalizeErr != nil &&
			!errors.Is(finalizeErr, apperror.ErrAlreadyFinalized) {
			s.logger.ErrorContext(
				ctx,
				"synchronous charge succeeded but finalization failed, running compensating refund",
				slog.String("payment_id", p.ID.String()),
				slog.String("order_id", params.OrderID.String()),
				slog.String("error", finalizeErr.Error()),
			)
			s.runCompensatingRefund(ctx, finalizeJob)
		}
	case string(StatusPending):
		if resp.PaymentURL != "" {
			if err := s.repo.UpdatePaymentURL(ctx, p.ID, resp.PaymentURL); err != nil {
				s.logger.ErrorContext(ctx, "failed to update payment url", slog.String("error", err.Error()))
			}
			result.PaymentURL = resp.PaymentURL
		}
	default:
		// Must not fall through to nil: that would make a declined charge look like
		// a success. The order stays awaiting_payment for retry or expiry.
		s.logger.WarnContext(
			ctx,
			"gateway declined charge synchronously",
			slog.String(
				"payment_id",
				p.ID.String(),
			),
			slog.String("order_id", params.OrderID.String()),
			slog.String("gateway_status", resp.Status),
		)
		return result, fmt.Errorf("%w: payment was declined", apperror.ErrBadRequest)
	}

	return result, nil
}

// Process owns its own retry and status bookkeeping. A returned error is only
// for the runner to log: the backoff is already persisted here.
func (s *Service) Process(ctx context.Context, job Job) error {
	// Only here: Charge and ProcessWebhook reach the same functions with a synthetic
	// Job that has no ID, so they must not set this.
	ctx = logger.WithAttrs(ctx, slog.String("job_id", job.ID.String()))

	switch job.Action {
	case ActionCharge:
		return s.processChargeJob(ctx, job)
	case ActionRefund:
		return s.processRefundJob(ctx, job)
	default:
		s.logger.ErrorContext(ctx, "unknown job action", slog.String("action", string(job.Action)))
		return fmt.Errorf("unknown job action: %s", job.Action)
	}
}

// FinalizePaymentSuccess marks the payment and order paid and deducts stock in
// one transaction.
//
// The MarkJobCompleted(job.ID) calls below are deliberate no-ops for two of the
// three callers, not lost writes: InitiatePayment and the webhook have no
// persisted job row and pass a synthetic Job whose id is uuid.Nil, so the
// UPDATE matches zero rows. Worth stating because MarkJobCompleted discards its
// rows-affected count, so nothing tells the two cases apart at runtime.
//
//nolint:gocognit // single finalize CAS with idempotent already-finalized and late-charge-on-terminal-order branches; funlen counts golines' wrapping, not added logic (78 lines before this commit's reformat, 108 after)
func (s *Service) FinalizePaymentSuccess(ctx context.Context, job Job) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		orderSnap, err := s.orderGet.GetByID(txCtx, job.OrderID)
		if err != nil {
			return fmt.Errorf("getting order for verification: %w", err)
		}

		p, err := s.repo.GetByID(txCtx, job.PaymentID)
		if err != nil {
			return fmt.Errorf("getting payment for verification: %w", err)
		}

		// ErrAmountMismatch even on a currency disagreement: the fact worth reporting
		// is that the charge does not match the order.
		if !p.Amount.Equal(orderSnap.Total) {
			return apperror.ErrAmountMismatch
		}

		paymentErr := s.repo.MarkPaid(txCtx, job.PaymentID,
			[]Status{StatusPending, StatusProcessing, StatusRequiresReview, StatusCancelled})

		orderErr := s.orders.MarkPaid(txCtx, job.OrderID)

		if paymentErr != nil && orderErr != nil {
			s.logger.InfoContext(txCtx, "job completed: already finalized by external actor (webhook)",
				slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()))
			if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
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
			if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRequiresReview,
				[]Status{StatusSuccess}); statusErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to update payment status to requires_review",
					slog.String("payment_id", job.PaymentID.String()),
					slog.String("error", statusErr.Error()),
				)
			}
			if orderStatusErr := s.orders.MarkFulfillmentFailedAfterCharge(txCtx, job.OrderID); orderStatusErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to update order status to fulfillment_failed",
					slog.String("order_id", job.OrderID.String()),
					slog.String("error", orderStatusErr.Error()),
				)
			}

			refundJob := &Job{
				PaymentID:   job.PaymentID,
				OrderID:     job.OrderID,
				Action:      ActionRefund,
				Status:      JobStatusPending,
				NextRetryAt: time.Now(),
			}
			if createErr := s.repo.CreateJob(txCtx, refundJob); createErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to create refund job",
					slog.String("order_id", job.OrderID.String()),
					slog.String("error", createErr.Error()),
				)
			}
			if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to mark job completed",
					slog.String("error", markErr.Error()),
				)
			}
			return nil
		}

		items, err := s.orderItems.ListItemsByOrderID(txCtx, job.OrderID)
		if err != nil {
			return fmt.Errorf("listing order items: %w", err)
		}

		if err := s.inventory.DeductBatch(txCtx, toInventoryChanges(items)); err != nil {
			return fmt.Errorf("deducting inventory: %w", err)
		}

		if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to mark job completed",
				slog.String("error", markErr.Error()),
			)
		}
		return nil
	})
}

//nolint:gocognit,funlen // resolves the payment then dispatches success/failed/cancelled/expired event branches; funlen counts golines' wrapping, not added logic
func (s *Service) HandleWebhook(
	ctx context.Context,
	payload map[string]any,
) error {
	event, _ := payload["event"].(string)
	metadata, _ := payload["metadata"].(map[string]any)
	txnID, _ := payload["transaction_id"].(string)

	var p *Payment

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

	// requires_review means a compensating refund already owns this payment: a
	// replayed webhook would cancel its job or re-finalize the payment.
	if p.Status == StatusSuccess || p.Status == StatusRefunded || p.Status == StatusRequiresReview {
		return nil
	}

	switch event {
	case string(StatusSuccess):
		job := Job{
			PaymentID: p.ID,
			OrderID:   p.OrderID,
			Action:    ActionCharge,
		}
		if err := s.FinalizePaymentSuccess(ctx, job); err != nil {
			if errors.Is(err, apperror.ErrAlreadyFinalized) {
				break
			}
			// Funds are captured, so a 5xx here would leave money taken and the order
			// unpaid forever. Compensate, then ack so the gateway stops retrying into a
			// failure already handled.
			s.logger.ErrorContext(
				ctx,
				"webhook finalization failed, running compensating refund",
				slog.String(
					"payment_id",
					p.ID.String(),
				),
				slog.String("order_id", p.OrderID.String()),
				slog.String("error", err.Error()),
			)
			s.runCompensatingRefund(ctx, job)
			return nil
		}
		if err := s.repo.MarkJobCompletedByPaymentID(ctx, p.ID, ActionCharge); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to mark job completed by payment id",
				slog.String("payment_id", p.ID.String()),
				slog.String("error", err.Error()),
			)
		}

	case "failed", "cancelled", "expired":
		if err := s.repo.UpdateStatus(ctx, p.ID, StatusCancelled,
			[]Status{StatusPending, StatusProcessing}); err != nil {
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
		if err := s.repo.CancelJobsByOrderID(ctx, p.OrderID); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to cancel jobs",
				slog.String("order_id", p.OrderID.String()),
				slog.String("error", err.Error()),
			)
		}
		// The expiry sweep cannot touch a payment_processing order, so release here.
		// ErrBadRequest means a concurrent charge already paid it; leave it be.
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

func (s *Service) CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error {
	return s.repo.CancelJobsByOrderID(ctx, orderID)
}

func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) ([]Payment, int, error) {
	return s.repo.ListAdmin(ctx, params)
}

func (s *Service) GetByID(ctx context.Context, id uuid.UUID) (*Payment, error) {
	return s.repo.GetByID(ctx, id)
}

func (s *Service) Refund(ctx context.Context, paymentID uuid.UUID) error {
	p, err := s.repo.GetByID(ctx, paymentID)
	if err != nil {
		return err
	}

	if p.Status != StatusSuccess && p.Status != StatusRequiresReview {
		return fmt.Errorf("%w: payment is not refundable", apperror.ErrBadRequest)
	}

	// The refund worker recomputes release-vs-restock from the order when it runs,
	// so enqueue is just intent — no need to resolve the inventory action here.
	job := &Job{
		PaymentID:   paymentID,
		OrderID:     p.OrderID,
		Action:      ActionRefund,
		Status:      JobStatusPending,
		NextRetryAt: time.Now(),
	}

	return s.repo.CreateJob(ctx, job)
}

func (s *Service) processChargeJob(ctx context.Context, job Job) error {
	err := s.orders.MarkPaymentProcessing(ctx, job.OrderID)
	if err != nil {
		s.logger.WarnContext(ctx, "charge job cancelled: order not in expected state",
			slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()))
		job.Status = JobStatusCancelled
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
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

	chargeReq := ChargeRequest{
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
			slog.Int(
				"attempt",
				job.Attempts,
			),
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
	case string(StatusSuccess):
		s.logger.InfoContext(ctx, "charge succeeded",
			slog.String("order_id", job.OrderID.String()), slog.String("payment_id", job.PaymentID.String()),
			slog.String("gateway_txn_id", resp.TransactionID), slog.Int("attempt", job.Attempts))

		if finalizeErr := s.FinalizePaymentSuccess(ctx, job); finalizeErr != nil {
			if errors.Is(finalizeErr, apperror.ErrAlreadyFinalized) {
				s.logger.InfoContext(ctx, "charge job: payment already finalized externally",
					slog.String("order_id", job.OrderID.String()))
				return nil
			}
			s.logger.ErrorContext(ctx, "finalization failed, running compensating flow",
				slog.String("order_id", job.OrderID.String()), slog.String("error", finalizeErr.Error()))
			s.runCompensatingRefund(ctx, job)
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

func (s *Service) handleChargeFailure(ctx context.Context, job *Job, lastError string) {
	job.LastError = lastError

	if err := s.orders.MarkAwaitingPayment(ctx, job.OrderID); err != nil {
		s.logger.ErrorContext(ctx, "failed to CAS order back to awaiting_payment",
			slog.String("order_id", job.OrderID.String()), slog.String("error", err.Error()))
	}

	if job.Attempts >= job.MaxAttempts {
		job.Status = JobStatusFailed
		job.LockedUntil = nil
	} else {
		job.Status = JobStatusPending
		job.LockedUntil = nil
		backoff := time.Duration(1<<min(max(job.Attempts, 0), 30)) * time.Second
		jitter := time.Duration(
			rand.N(int64(backoff / jitterDivisor)), //nolint:gosec // jitter doesn't need crypto randomness
		)
		nextRetry := time.Now().Add(backoff + jitter)
		job.NextRetryAt = nextRetry
	}

	if err := s.repo.UpdateJob(ctx, job); err != nil {
		s.logger.ErrorContext(ctx, "failed to update job after failure",
			slog.String("error", err.Error()))
	}
}

func (s *Service) runCompensatingRefund(ctx context.Context, job Job) {
	txErr := s.tx.Run(ctx, func(txCtx context.Context) error {
		if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRequiresReview,
			[]Status{StatusPending, StatusProcessing, StatusSuccess}); statusErr != nil {
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

		refundJob := &Job{
			PaymentID:   job.PaymentID,
			OrderID:     job.OrderID,
			Action:      ActionRefund,
			Status:      JobStatusPending,
			NextRetryAt: time.Now(),
		}
		return s.repo.CreateJob(txCtx, refundJob)
	})
	if txErr != nil {
		s.logger.ErrorContext(ctx, "compensating refund failed",
			slog.String("order_id", job.OrderID.String()), slog.String("error", txErr.Error()))
	}
}

//nolint:gocognit,funlen // refund retry/backoff bookkeeping plus the gateway-call-then-commit finalization; funlen counts golines' wrapping, not added logic
func (s *Service) processRefundJob(ctx context.Context, job Job) error {
	p, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		s.logger.ErrorContext(
			ctx,
			"failed to get payment for refund",
			slog.String("error", err.Error()),
		)
		return fmt.Errorf("getting payment for refund: %w", err)
	}

	if p.Status != StatusSuccess && p.Status != StatusRequiresReview {
		s.logger.WarnContext(ctx, "refund job cancelled: payment not refundable",
			slog.String("payment_status", string(p.Status)))
		job.Status = JobStatusCancelled
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
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

	// No currency crosses: the gateway refunds whatever TransactionID was.
	resp, gwErr := s.gateway.Refund(ctx, RefundRequest{
		// Keyed on the payment id, so a job re-claimed after a crash between this call
		// and the commit reuses the key and the gateway dedupes it.
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
			job.Status = JobStatusFailed
		} else {
			job.Status = JobStatusPending
			backoff := time.Duration(1<<min(max(job.Attempts, 0), 30)) * time.Second
			jitter := time.Duration(
				rand.N(int64(backoff / jitterDivisor)), //nolint:gosec // jitter doesn't need crypto randomness
			)
			job.NextRetryAt = time.Now().Add(backoff + jitter)
		}
		job.LockedUntil = nil
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
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
		// Read BEFORE the flip to refunded: StockDeducted picks restock vs release,
		// and StockReversed says the hold is already unwound -- reversing twice would
		// steal another order's reservation.
		orderSnap, snapErr := s.orderGet.GetByID(txCtx, job.OrderID)
		if snapErr != nil {
			return fmt.Errorf("getting order for refund: %w", snapErr)
		}

		if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}); statusErr != nil {
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

		items, listErr := s.orderItems.ListItemsByOrderID(txCtx, job.OrderID)
		if listErr != nil {
			return listErr
		}
		switch {
		case orderSnap.Dispatched:
			// The goods already left the warehouse, so refund the money but do NOT
			// restock — adding shipped units back to sellable stock would oversell.
			s.logger.InfoContext(txCtx, "refund: skipping inventory restock for dispatched order",
				slog.String("order_id", job.OrderID.String()), slog.String("order_status", orderSnap.Status))
		case len(items) > 0 && !orderSnap.StockReversed:
			// Inventory owns release-vs-restock; we pass the order's persisted fact.
			if restoreErr := s.inventoryRestorer.Restore(
				txCtx,
				toInventoryChanges(items),
				orderSnap.StockDeducted,
			); restoreErr != nil {
				s.logger.ErrorContext(txCtx, "failed to restore inventory on refund",
					slog.String("order_id", job.OrderID.String()), slog.String("error", restoreErr.Error()))
			}
		}

		if s.couponReleaser != nil && orderSnap.CouponCode != "" {
			if releaseErr := s.couponReleaser.Release(txCtx, job.OrderID); releaseErr != nil {
				s.logger.WarnContext(
					txCtx,
					"failed to release coupon on refund",
					slog.String("error", releaseErr.Error()),
				)
			}
		}

		if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
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
