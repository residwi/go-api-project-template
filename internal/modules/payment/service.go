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

	// ChargeRequest is the external gateway's wire contract, and it keeps a bare
	// int64 beside a currency string because that is the shape the gateway
	// publishes -- see the JSON_TAG_ALLOWLIST entry in scripts/check-boundaries.sh.
	// Unpairing the Money here is the seam, not a leak: the pairing holds
	// everywhere this system owns the type, and stops at the point where someone
	// else's API begins.
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
		s.logger.ErrorContext(ctx, "gateway charge failed",
			slog.Any("payment_id", p.ID), slog.Any("order_id", params.OrderID), slog.Any("error", err))
		return InitiatePaymentResult{PaymentID: p.ID}, fmt.Errorf("gateway charge: %w", err)
	}

	respJSON, _ := json.Marshal(resp)
	if err := s.repo.UpdateGateway(ctx, p.ID, resp.TransactionID, respJSON); err != nil {
		s.logger.ErrorContext(ctx, "failed to update gateway info", slog.Any("error", err))
	}

	result := InitiatePaymentResult{PaymentID: p.ID}

	switch resp.Status {
	case string(StatusSuccess):
		result.Charged = true
		// The gateway captured funds synchronously (e.g. a card charge with a
		// PaymentMethodID), so finalize the order NOW — mark payment+order paid and
		// deduct inventory — instead of leaving it in awaiting_payment for a webhook
		// or job that never comes. Mirrors the webhook success path.
		finalizeJob := Job{PaymentID: p.ID, OrderID: params.OrderID, Action: ActionCharge}
		if finalizeErr := s.FinalizePaymentSuccess(
			ctx,
			finalizeJob,
		); finalizeErr != nil &&
			!errors.Is(finalizeErr, apperror.ErrAlreadyFinalized) {
			s.logger.ErrorContext(
				ctx,
				"synchronous charge succeeded but finalization failed, running compensating refund",
				slog.Any("payment_id", p.ID),
				slog.Any("order_id", params.OrderID),
				slog.Any("error", finalizeErr),
			)
			s.runCompensatingRefund(ctx, finalizeJob)
		}
	case string(StatusPending):
		if resp.PaymentURL != "" {
			if err := s.repo.UpdatePaymentURL(ctx, p.ID, resp.PaymentURL); err != nil {
				s.logger.ErrorContext(ctx, "failed to update payment url", slog.Any("error", err))
			}
			result.PaymentURL = resp.PaymentURL
		}
	default:
		// A synchronous decline (non-success, non-pending) must surface as an
		// error, not be swallowed into the nil-error fall-through that would make a
		// declined charge look like success. Handled like the gateway-error path
		// above; the order stays awaiting_payment for retry/expiry.
		s.logger.WarnContext(ctx, "gateway declined charge synchronously",
			slog.Any("payment_id", p.ID), slog.Any("order_id", params.OrderID), slog.Any("gateway_status", resp.Status))
		return result, fmt.Errorf("%w: payment was declined", apperror.ErrBadRequest)
	}

	return result, nil
}

// Process runs one payment job to a settled state, owning its own retry and
// status bookkeeping. It returns nil when the job is done (succeeded or already
// finalized) and a descriptive error when this attempt did not complete, purely
// so the runner can log it — the retry/backoff is already persisted here.
func (s *Service) Process(ctx context.Context, job Job) error {
	switch job.Action {
	case ActionCharge:
		return s.processChargeJob(ctx, job)
	case ActionRefund:
		return s.processRefundJob(ctx, job)
	default:
		s.logger.ErrorContext(ctx, "unknown job action", slog.Any("job_id", job.ID), slog.Any("action", job.Action))
		return fmt.Errorf("unknown job action: %s", job.Action)
	}
}

// FinalizePaymentSuccess marks the payment and order paid and deducts stock, in
// one transaction.
//
// Two of its three callers pass a **synthetic** Job carrying only PaymentID,
// OrderID and Action -- the synchronous-charge path (InitiatePayment) and the
// webhook -- because neither has a persisted job row to work from. Only the
// worker passes a Job with a real ID. That means the `MarkJobCompleted(job.ID)`
// calls below are deliberately no-ops for those two callers: the id is
// uuid.Nil, so the UPDATE matches zero rows. It is not a lost write. The
// webhook additionally calls MarkJobCompletedByPaymentID afterwards, and no
// charge job exists to complete in the first place -- every CreateJob call site
// in this package enqueues ActionRefund. See ARCHITECTURE-LIMITATIONS.md.
//
// Stated here because a zero-row UPDATE is invisible: MarkJobCompleted
// discards its rows-affected count, so nothing distinguishes "no such job" from
// "job completed" at runtime.
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

		// money.Money.Equal compares amount AND currency, which is exactly the
		// two-field check this replaces. apperror.ErrAmountMismatch is kept rather
		// than swapped for money.ErrCurrencyMismatch: the fact worth reporting is
		// that the charge does not match the order, and a currency disagreement here
		// is one way for that to be true, not a different failure.
		if !p.Amount.Equal(orderSnap.Total) {
			return apperror.ErrAmountMismatch
		}

		paymentErr := s.repo.MarkPaid(txCtx, job.PaymentID,
			[]Status{StatusPending, StatusProcessing, StatusRequiresReview, StatusCancelled})

		orderErr := s.orders.MarkPaid(txCtx, job.OrderID)

		if paymentErr != nil && orderErr != nil {
			s.logger.InfoContext(txCtx, "job completed: already finalized by external actor (webhook)",
				slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID), slog.Any("payment_id", job.PaymentID))
			if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to mark job completed",
					slog.Any("job_id", job.ID),
					slog.Any("error", markErr),
				)
			}
			return apperror.ErrAlreadyFinalized
		}

		if orderErr != nil {
			s.logger.ErrorContext(txCtx, "late payment success on terminal order, auto-refund enqueued",
				slog.Any("order_id", job.OrderID), slog.Any("payment_id", job.PaymentID),
				slog.Any("order_status", orderSnap.Status))
			if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRequiresReview,
				[]Status{StatusSuccess}); statusErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to update payment status to requires_review",
					slog.Any("payment_id", job.PaymentID),
					slog.Any("error", statusErr),
				)
			}
			if orderStatusErr := s.orders.MarkFulfillmentFailedAfterCharge(txCtx, job.OrderID); orderStatusErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to update order status to fulfillment_failed",
					slog.Any("order_id", job.OrderID),
					slog.Any("error", orderStatusErr),
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
					slog.Any("order_id", job.OrderID),
					slog.Any("error", createErr),
				)
			}
			if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to mark job completed",
					slog.Any("job_id", job.ID),
					slog.Any("error", markErr),
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
				slog.Any("job_id", job.ID),
				slog.Any("error", markErr),
			)
		}
		return nil
	})
}

//nolint:gocognit // resolves the payment then dispatches success/failed/cancelled/expired event branches; funlen counts golines' wrapping, not added logic
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
						slog.Any("payment_id", pid),
						slog.Any("error", getErr),
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
					slog.Any("txn_id", txnID),
					slog.Any("error", getErr),
				)
			}
		} else {
			p = found
		}
	}

	if p == nil {
		s.logger.ErrorContext(ctx, "webhook: unknown payment_id", slog.Any("payload_event", event))
		return nil
	}

	// requires_review means a compensating refund already owns this payment, so a
	// late/replayed webhook must not re-drive it (a failed event would cancel the
	// in-flight refund job; a duplicate success would re-finalize it).
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
			// The gateway has already captured funds, so a finalization failure
			// (e.g. inventory deduction failed) must not just 5xx and leave money
			// captured with the order unpaid forever. Compensate the same way the
			// worker charge path does: flag the order fulfillment_failed and enqueue
			// a refund. Ack the webhook so the gateway stops retrying into a failure
			// we've already handled.
			s.logger.ErrorContext(ctx, "webhook finalization failed, running compensating refund",
				slog.Any("payment_id", p.ID), slog.Any("order_id", p.OrderID), slog.Any("error", err))
			s.runCompensatingRefund(ctx, job)
			return nil
		}
		if err := s.repo.MarkJobCompletedByPaymentID(ctx, p.ID, ActionCharge); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to mark job completed by payment id",
				slog.Any("payment_id", p.ID),
				slog.Any("error", err),
			)
		}

	case "failed", "cancelled", "expired":
		if err := s.repo.UpdateStatus(ctx, p.ID, StatusCancelled,
			[]Status{StatusPending, StatusProcessing}); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to update payment status to cancelled",
				slog.Any("payment_id", p.ID),
				slog.Any("error", err),
			)
		}
		if err := s.repo.ClearPaymentURL(ctx, p.ID); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to clear payment url",
				slog.Any("payment_id", p.ID),
				slog.Any("error", err),
			)
		}
		if err := s.repo.CancelJobsByOrderID(ctx, p.OrderID); err != nil {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to cancel jobs",
				slog.Any("order_id", p.OrderID),
				slog.Any("error", err),
			)
		}
		// Cancel the order and release its reserved stock now rather than leaving it
		// reserved until the expiry sweep (which can't touch a payment_processing
		// order at all). ErrBadRequest means the order is no longer cancellable
		// (e.g. a concurrent charge already paid it) — leave it for that flow.
		if err := s.orders.CancelUnpaid(ctx, p.OrderID); err != nil && !errors.Is(err, apperror.ErrBadRequest) {
			s.logger.ErrorContext(
				ctx,
				"webhook: failed to cancel order after payment failure",
				slog.Any("order_id", p.OrderID),
				slog.Any("error", err),
			)
		}
		s.logger.InfoContext(ctx, "webhook payment failed",
			slog.Any("payment_id", p.ID), slog.Any("gateway_event", event))
	}

	return nil
}

func (s *Service) CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error {
	return s.repo.CancelJobsByOrderID(ctx, orderID)
}

// ListAdmin lists payments for the admin dashboard. It delegates straight to
// the repository; there is no business logic beyond the query itself.
func (s *Service) ListAdmin(ctx context.Context, params AdminListParams) ([]Payment, int, error) {
	return s.repo.ListAdmin(ctx, params)
}

// GetByID fetches a single payment by ID, delegating straight to the repository.
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
			slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID), slog.Any("payment_id", job.PaymentID))
		job.Status = JobStatusCancelled
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
			s.logger.ErrorContext(
				ctx,
				"failed to update cancelled job",
				slog.Any("job_id", job.ID),
				slog.Any("error", updateErr),
			)
		}
		return fmt.Errorf("charge job cancelled: order %s not in expected state", job.OrderID)
	}

	p, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get payment for job", slog.Any("job_id", job.ID), slog.Any("error", err))
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
		s.logger.WarnContext(ctx, "charge failed",
			slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID), slog.Any("payment_id", job.PaymentID),
			slog.Any("attempt", job.Attempts), slog.Any("max_attempts", job.MaxAttempts), slog.Any("error", gwErr))

		s.handleChargeFailure(ctx, &job, gwErr.Error())
		return fmt.Errorf("charge failed: %w", gwErr)
	}

	respJSON, _ := json.Marshal(resp)
	if updateErr := s.repo.UpdateGateway(ctx, p.ID, resp.TransactionID, respJSON); updateErr != nil {
		s.logger.ErrorContext(
			ctx,
			"failed to update gateway info",
			slog.Any("job_id", job.ID),
			slog.Any("error", updateErr),
		)
	}

	switch resp.Status {
	case string(StatusSuccess):
		s.logger.InfoContext(ctx, "charge succeeded",
			slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID), slog.Any("payment_id", job.PaymentID),
			slog.Any("gateway_txn_id", resp.TransactionID), slog.Any("attempt", job.Attempts))

		if finalizeErr := s.FinalizePaymentSuccess(ctx, job); finalizeErr != nil {
			if errors.Is(finalizeErr, apperror.ErrAlreadyFinalized) {
				s.logger.InfoContext(ctx, "charge job: payment already finalized externally",
					slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID))
				return nil
			}
			s.logger.ErrorContext(ctx, "finalization failed, running compensating flow",
				slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID), slog.Any("error", finalizeErr))
			s.runCompensatingRefund(ctx, job)
		}
		return nil

	default:
		s.logger.WarnContext(ctx, "charge returned non-success",
			slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID), slog.Any("status", resp.Status),
			slog.Any("attempt", job.Attempts))
		s.handleChargeFailure(ctx, &job, fmt.Sprintf("gateway returned status: %s", resp.Status))
		return fmt.Errorf("charge returned non-success status: %s", resp.Status)
	}
}

func (s *Service) handleChargeFailure(ctx context.Context, job *Job, lastError string) {
	job.LastError = lastError

	if err := s.orders.MarkAwaitingPayment(ctx, job.OrderID); err != nil {
		s.logger.ErrorContext(ctx, "failed to CAS order back to awaiting_payment",
			slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID), slog.Any("error", err))
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
			slog.Any("job_id", job.ID), slog.Any("error", err))
	}
}

func (s *Service) runCompensatingRefund(ctx context.Context, job Job) {
	txErr := s.tx.Run(ctx, func(txCtx context.Context) error {
		if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRequiresReview,
			[]Status{StatusPending, StatusProcessing, StatusSuccess}); statusErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update payment status in compensating refund",
				slog.Any("payment_id", job.PaymentID),
				slog.Any("error", statusErr),
			)
		}
		if orderErr := s.orders.MarkFulfillmentFailedCompensating(txCtx, job.OrderID); orderErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update order status in compensating refund",
				slog.Any("order_id", job.OrderID),
				slog.Any("error", orderErr),
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
			slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID), slog.Any("error", txErr))
	}
}

//nolint:gocognit,funlen // refund retry/backoff bookkeeping plus the gateway-call-then-commit finalization; funlen counts golines' wrapping, not added logic
func (s *Service) processRefundJob(ctx context.Context, job Job) error {
	p, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		s.logger.ErrorContext(
			ctx,
			"failed to get payment for refund",
			slog.Any("job_id", job.ID),
			slog.Any("error", err),
		)
		return fmt.Errorf("getting payment for refund: %w", err)
	}

	if p.Status != StatusSuccess && p.Status != StatusRequiresReview {
		s.logger.WarnContext(ctx, "refund job cancelled: payment not refundable",
			slog.Any("job_id", job.ID), slog.Any("payment_status", p.Status))
		job.Status = JobStatusCancelled
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
			s.logger.ErrorContext(
				ctx,
				"failed to update cancelled refund job",
				slog.Any("job_id", job.ID),
				slog.Any("error", updateErr),
			)
		}
		return fmt.Errorf("refund job cancelled: payment %s not refundable", job.PaymentID)
	}

	s.logger.InfoContext(
		ctx,
		"processing refund",
		slog.Any("job_id", job.ID),
		slog.Any("order_id", job.OrderID),
		slog.Any("payment_id", job.PaymentID),
		slog.Any(
			"gateway_txn_id",
			p.GatewayTxnID,
		),
		slog.Any("amount", p.Amount.Amount),
		slog.Any("currency", p.Amount.Currency),
	)

	// RefundRequest carries no currency at all -- the gateway identifies the
	// original charge by TransactionID and refunds in whatever that was
	// denominated in. Only the amount crosses, which is the same seam as
	// ChargeRequest above and not a place the pairing is lost.
	resp, gwErr := s.gateway.Refund(ctx, RefundRequest{
		// Key on the payment id: a payment is refunded once, so a job re-claimed
		// after a crash between this call and the commit reuses the same key and
		// the gateway dedupes it instead of refunding twice.
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
			slog.Any(
				"job_id",
				job.ID,
			),
			slog.Any("order_id", job.OrderID),
			slog.Any("attempt", job.Attempts),
			slog.Any("error", gwErr),
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
				slog.Any("job_id", job.ID),
				slog.Any("error", updateErr),
			)
		}
		return fmt.Errorf("refund failed: %w", gwErr)
	}

	s.logger.InfoContext(ctx, "refund completed",
		slog.Any("job_id", job.ID), slog.Any("order_id", job.OrderID), slog.Any("payment_id", job.PaymentID),
		slog.Any("refund_id", resp.RefundID))

	txErr := s.tx.Run(ctx, func(txCtx context.Context) error {
		// Capture the order's persisted stock state BEFORE flipping it to refunded:
		// StockDeducted chooses restock vs release, and StockReversed tells us the
		// hold was already unwound (e.g. the order was cancelled/expired before a
		// late charge landed), so reversing again would double-release and steal
		// another order's reservation.
		orderSnap, snapErr := s.orderGet.GetByID(txCtx, job.OrderID)
		if snapErr != nil {
			return fmt.Errorf("getting order for refund: %w", snapErr)
		}

		if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}); statusErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update payment status to refunded",
				slog.Any("payment_id", job.PaymentID),
				slog.Any("error", statusErr),
			)
		}
		if orderStatusErr := s.orders.MarkRefunded(txCtx, job.OrderID); orderStatusErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update order status to refunded",
				slog.Any("order_id", job.OrderID),
				slog.Any("error", orderStatusErr),
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
				slog.Any("order_id", job.OrderID), slog.Any("order_status", orderSnap.Status))
		case len(items) > 0 && !orderSnap.StockReversed:
			// Inventory owns release-vs-restock; we pass the order's persisted fact.
			if restoreErr := s.inventoryRestorer.Restore(
				txCtx,
				toInventoryChanges(items),
				orderSnap.StockDeducted,
			); restoreErr != nil {
				s.logger.ErrorContext(txCtx, "failed to restore inventory on refund",
					slog.Any("order_id", job.OrderID), slog.Any("error", restoreErr))
			}
		}

		if s.couponReleaser != nil && orderSnap.CouponCode != "" {
			if releaseErr := s.couponReleaser.Release(txCtx, job.OrderID); releaseErr != nil {
				s.logger.WarnContext(txCtx, "failed to release coupon on refund", slog.Any("error", releaseErr))
			}
		}

		if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to mark refund job completed",
				slog.Any("job_id", job.ID),
				slog.Any("error", markErr),
			)
		}
		return nil
	})

	if txErr != nil {
		return fmt.Errorf("refund finalization failed: %w", txErr)
	}
	return nil
}
