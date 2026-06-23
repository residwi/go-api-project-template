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
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	gateway "github.com/residwi/go-api-project-template/internal/platform/payment"
)

const jitterDivisor = 2

// OrderUpdater drives order-status changes from the payment domain via intent
// methods, so payment never imports the order package; the wiring adapter maps
// each method to the corresponding order.Transition (which owns the allowed-from
// set).
type OrderUpdater interface {
	MarkPaymentProcessing(ctx context.Context, orderID uuid.UUID) error
	MarkAwaitingPayment(ctx context.Context, orderID uuid.UUID) error
	MarkPaid(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedAfterCharge(ctx context.Context, orderID uuid.UUID) error
	MarkFulfillmentFailedCompensating(ctx context.Context, orderID uuid.UUID) error
	MarkRefunded(ctx context.Context, orderID uuid.UUID) error
	// CancelUnpaid cancels an order whose payment terminally failed and releases
	// its reserved stock and coupon. Returns a wrapped core.ErrBadRequest when the
	// order is no longer cancellable (e.g. already paid by a concurrent charge).
	CancelUnpaid(ctx context.Context, orderID uuid.UUID) error
}

type OrderItemDTO struct {
	ProductID uuid.UUID
	Quantity  int
}

type OrderItemsGetter interface {
	ListItemsByOrderID(ctx context.Context, orderID uuid.UUID) ([]OrderItemDTO, error)
}

type OrderSnapshot struct {
	TotalAmount int64
	Currency    string
	Status      string
	CouponCode  string
	// StockDeducted reports whether the order's inventory was deducted from
	// stock (vs only reserved); StockReversed reports whether the hold was
	// already released/restocked. The order module owns these facts (persisted,
	// not re-derived from Status); payment uses them to choose restock vs release
	// and to skip a reversal that already happened (avoiding a double release).
	StockDeducted bool
	StockReversed bool
	// Dispatched reports whether the order's goods have physically left the
	// warehouse (shipped/delivered). The order module owns the mapping from its
	// status enum; payment reads this flag to skip restocking on refund rather
	// than re-deriving order semantics from a status string it can't import.
	Dispatched bool
}

type OrderGetter interface {
	GetByID(ctx context.Context, orderID uuid.UUID) (OrderSnapshot, error)
}

// InventoryChange is one product/quantity pair for a batched inventory op.
type InventoryChange struct {
	ProductID uuid.UUID
	Quantity  int
}

type InventoryDeductor interface {
	DeductBatch(ctx context.Context, items []InventoryChange) error
}

type InventoryRestorer interface {
	// Restore reverses an order's inventory effect; wasDeducted selects release
	// vs restock. Inventory owns that choice — payment only supplies the order's
	// fact (computed from its snapshot), not the mechanics.
	Restore(ctx context.Context, items []InventoryChange, wasDeducted bool) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}

func toInventoryChanges(items []OrderItemDTO) []InventoryChange {
	changes := make([]InventoryChange, len(items))
	for i, it := range items {
		changes[i] = InventoryChange(it)
	}
	return changes
}

type Service struct {
	repo              Repository
	pool              *pgxpool.Pool
	gateway           gateway.Gateway
	orders            OrderUpdater
	orderGet          OrderGetter
	orderItems        OrderItemsGetter
	inventory         InventoryDeductor
	inventoryRestorer InventoryRestorer
	couponReleaser    CouponReleaser
}

func NewService(
	repo Repository,
	pool *pgxpool.Pool,
	gw gateway.Gateway,
	orders OrderUpdater,
	orderGet OrderGetter,
	orderItems OrderItemsGetter,
	inventory InventoryDeductor,
	inventoryRestorer InventoryRestorer,
	couponReleaser CouponReleaser,
) *Service {
	return &Service{
		repo:              repo,
		pool:              pool,
		gateway:           gw,
		orders:            orders,
		orderGet:          orderGet,
		orderItems:        orderItems,
		inventory:         inventory,
		inventoryRestorer: inventoryRestorer,
		couponReleaser:    couponReleaser,
	}
}

type InitiatePaymentParams struct {
	OrderID         uuid.UUID
	Amount          int64
	Currency        string
	PaymentMethodID string
}

type InitiatePaymentResult struct {
	PaymentID  uuid.UUID
	PaymentURL string
	Charged    bool
}

func (s *Service) InitiatePayment(ctx context.Context, params InitiatePaymentParams) (InitiatePaymentResult, error) {
	existing, err := s.repo.GetActiveByOrderID(ctx, params.OrderID)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		return InitiatePaymentResult{}, err
	}

	var p *Payment
	if !errors.Is(err, core.ErrNotFound) {
		p = existing
	} else {
		p = &Payment{
			OrderID:         params.OrderID,
			Amount:          params.Amount,
			Currency:        params.Currency,
			Status:          StatusPending,
			PaymentMethodID: params.PaymentMethodID,
		}
		if createErr := s.repo.Create(ctx, p); createErr != nil {
			return InitiatePaymentResult{}, createErr
		}
	}

	chargeReq := gateway.ChargeRequest{
		IdempotencyKey:  p.ID.String(),
		OrderID:         params.OrderID.String(),
		Amount:          params.Amount,
		Currency:        params.Currency,
		PaymentMethodID: params.PaymentMethodID,
		Metadata:        map[string]string{"payment_id": p.ID.String()},
	}

	resp, err := s.gateway.Charge(ctx, chargeReq)
	if err != nil {
		slog.ErrorContext(ctx, "gateway charge failed",
			"payment_id", p.ID, "order_id", params.OrderID, "error", err)
		return InitiatePaymentResult{PaymentID: p.ID}, fmt.Errorf("gateway charge: %w", err)
	}

	respJSON, _ := json.Marshal(resp)
	if err := s.repo.UpdateGateway(ctx, p.ID, resp.TransactionID, respJSON); err != nil {
		slog.ErrorContext(ctx, "failed to update gateway info", "error", err)
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
		if finalizeErr := s.FinalizePaymentSuccess(ctx, finalizeJob); finalizeErr != nil && !errors.Is(finalizeErr, core.ErrAlreadyFinalized) {
			slog.ErrorContext(ctx, "synchronous charge succeeded but finalization failed, running compensating refund",
				"payment_id", p.ID, "order_id", params.OrderID, "error", finalizeErr)
			s.runCompensatingRefund(ctx, finalizeJob)
		}
	case string(StatusPending):
		if resp.PaymentURL != "" {
			if err := s.repo.UpdatePaymentURL(ctx, p.ID, resp.PaymentURL); err != nil {
				slog.ErrorContext(ctx, "failed to update payment url", "error", err)
			}
			result.PaymentURL = resp.PaymentURL
		}
	default:
		// A synchronous decline (non-success, non-pending) must surface as an
		// error, not be swallowed into the nil-error fall-through that would make a
		// declined charge look like success. Handled like the gateway-error path
		// above; the order stays awaiting_payment for retry/expiry.
		slog.WarnContext(ctx, "gateway declined charge synchronously",
			"payment_id", p.ID, "order_id", params.OrderID, "gateway_status", resp.Status)
		return result, fmt.Errorf("%w: payment was declined", core.ErrBadRequest)
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
		slog.ErrorContext(ctx, "unknown job action", "job_id", job.ID, "action", job.Action)
		return fmt.Errorf("unknown job action: %s", job.Action)
	}
}

func (s *Service) processChargeJob(ctx context.Context, job Job) error {
	err := s.orders.MarkPaymentProcessing(ctx, job.OrderID)
	if err != nil {
		slog.WarnContext(ctx, "charge job cancelled: order not in expected state",
			"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID)
		job.Status = JobStatusCancelled
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
			slog.ErrorContext(ctx, "failed to update cancelled job", "job_id", job.ID, "error", updateErr)
		}
		return fmt.Errorf("charge job cancelled: order %s not in expected state", job.OrderID)
	}

	p, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get payment for job", "job_id", job.ID, "error", err)
		return fmt.Errorf("getting payment for job: %w", err)
	}

	chargeReq := gateway.ChargeRequest{
		IdempotencyKey:  p.ID.String(),
		OrderID:         job.OrderID.String(),
		Amount:          p.Amount,
		Currency:        p.Currency,
		PaymentMethodID: p.PaymentMethodID,
		Metadata:        map[string]string{"payment_id": p.ID.String()},
	}

	resp, gwErr := s.gateway.Charge(ctx, chargeReq)

	job.Attempts++

	if gwErr != nil {
		slog.WarnContext(ctx, "charge failed",
			"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID,
			"attempt", job.Attempts, "max_attempts", job.MaxAttempts, "error", gwErr)

		s.handleChargeFailure(ctx, &job, gwErr.Error())
		return fmt.Errorf("charge failed: %w", gwErr)
	}

	respJSON, _ := json.Marshal(resp)
	if updateErr := s.repo.UpdateGateway(ctx, p.ID, resp.TransactionID, respJSON); updateErr != nil {
		slog.ErrorContext(ctx, "failed to update gateway info", "job_id", job.ID, "error", updateErr)
	}

	switch resp.Status {
	case string(StatusSuccess):
		slog.InfoContext(ctx, "charge succeeded",
			"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID,
			"gateway_txn_id", resp.TransactionID, "attempt", job.Attempts)

		if finalizeErr := s.FinalizePaymentSuccess(ctx, job); finalizeErr != nil {
			if errors.Is(finalizeErr, core.ErrAlreadyFinalized) {
				slog.InfoContext(ctx, "charge job: payment already finalized externally",
					"job_id", job.ID, "order_id", job.OrderID)
				return nil
			}
			slog.ErrorContext(ctx, "finalization failed, running compensating flow",
				"job_id", job.ID, "order_id", job.OrderID, "error", finalizeErr)
			s.runCompensatingRefund(ctx, job)
		}
		return nil

	default:
		slog.WarnContext(ctx, "charge returned non-success",
			"job_id", job.ID, "order_id", job.OrderID, "status", resp.Status,
			"attempt", job.Attempts)
		s.handleChargeFailure(ctx, &job, fmt.Sprintf("gateway returned status: %s", resp.Status))
		return fmt.Errorf("charge returned non-success status: %s", resp.Status)
	}
}

func (s *Service) handleChargeFailure(ctx context.Context, job *Job, lastError string) {
	job.LastError = lastError

	if err := s.orders.MarkAwaitingPayment(ctx, job.OrderID); err != nil {
		slog.ErrorContext(ctx, "failed to CAS order back to awaiting_payment",
			"job_id", job.ID, "order_id", job.OrderID, "error", err)
	}

	if job.Attempts >= job.MaxAttempts {
		job.Status = JobStatusFailed
		job.LockedUntil = nil
	} else {
		job.Status = JobStatusPending
		job.LockedUntil = nil
		backoff := time.Duration(1<<min(max(job.Attempts, 0), 30)) * time.Second
		jitter := time.Duration(rand.N(int64(backoff / jitterDivisor))) //nolint:gosec // jitter doesn't need crypto randomness
		nextRetry := time.Now().Add(backoff + jitter)
		job.NextRetryAt = nextRetry
	}

	if err := s.repo.UpdateJob(ctx, job); err != nil {
		slog.ErrorContext(ctx, "failed to update job after failure",
			"job_id", job.ID, "error", err)
	}
}

//nolint:gocognit // single finalize CAS with idempotent already-finalized and late-charge-on-terminal-order branches
func (s *Service) FinalizePaymentSuccess(ctx context.Context, job Job) error {
	return database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		orderSnap, err := s.orderGet.GetByID(txCtx, job.OrderID)
		if err != nil {
			return fmt.Errorf("getting order for verification: %w", err)
		}

		p, err := s.repo.GetByID(txCtx, job.PaymentID)
		if err != nil {
			return fmt.Errorf("getting payment for verification: %w", err)
		}

		if p.Amount != orderSnap.TotalAmount || p.Currency != orderSnap.Currency {
			return core.ErrAmountMismatch
		}

		paymentErr := s.repo.MarkPaid(txCtx, job.PaymentID,
			[]Status{StatusPending, StatusProcessing, StatusRequiresReview, StatusCancelled})

		orderErr := s.orders.MarkPaid(txCtx, job.OrderID)

		if paymentErr != nil && orderErr != nil {
			slog.InfoContext(txCtx, "job completed: already finalized by external actor (webhook)",
				"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID)
			if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				slog.ErrorContext(txCtx, "failed to mark job completed", "job_id", job.ID, "error", markErr)
			}
			return core.ErrAlreadyFinalized
		}

		if orderErr != nil {
			slog.ErrorContext(txCtx, "late payment success on terminal order, auto-refund enqueued",
				"order_id", job.OrderID, "payment_id", job.PaymentID,
				"order_status", orderSnap.Status)
			if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRequiresReview,
				[]Status{StatusSuccess}); statusErr != nil {
				slog.ErrorContext(txCtx, "failed to update payment status to requires_review", "payment_id", job.PaymentID, "error", statusErr)
			}
			if orderStatusErr := s.orders.MarkFulfillmentFailedAfterCharge(txCtx, job.OrderID); orderStatusErr != nil {
				slog.ErrorContext(txCtx, "failed to update order status to fulfillment_failed", "order_id", job.OrderID, "error", orderStatusErr)
			}

			refundJob := &Job{
				PaymentID:   job.PaymentID,
				OrderID:     job.OrderID,
				Action:      ActionRefund,
				Status:      JobStatusPending,
				NextRetryAt: time.Now(),
			}
			if createErr := s.repo.CreateJob(txCtx, refundJob); createErr != nil {
				slog.ErrorContext(txCtx, "failed to create refund job", "order_id", job.OrderID, "error", createErr)
			}
			if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				slog.ErrorContext(txCtx, "failed to mark job completed", "job_id", job.ID, "error", markErr)
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
			slog.ErrorContext(txCtx, "failed to mark job completed", "job_id", job.ID, "error", markErr)
		}
		return nil
	})
}

func (s *Service) runCompensatingRefund(ctx context.Context, job Job) {
	txErr := database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRequiresReview,
			[]Status{StatusPending, StatusProcessing, StatusSuccess}); statusErr != nil {
			slog.ErrorContext(txCtx, "failed to update payment status in compensating refund", "payment_id", job.PaymentID, "error", statusErr)
		}
		if orderErr := s.orders.MarkFulfillmentFailedCompensating(txCtx, job.OrderID); orderErr != nil {
			slog.ErrorContext(txCtx, "failed to update order status in compensating refund", "order_id", job.OrderID, "error", orderErr)
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
		slog.ErrorContext(ctx, "compensating refund failed",
			"job_id", job.ID, "order_id", job.OrderID, "error", txErr)
	}
}

//nolint:gocognit // refund retry/backoff bookkeeping plus the gateway-call-then-commit finalization
func (s *Service) processRefundJob(ctx context.Context, job Job) error {
	p, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get payment for refund", "job_id", job.ID, "error", err)
		return fmt.Errorf("getting payment for refund: %w", err)
	}

	if p.Status != StatusSuccess && p.Status != StatusRequiresReview {
		slog.WarnContext(ctx, "refund job cancelled: payment not refundable",
			"job_id", job.ID, "payment_status", p.Status)
		job.Status = JobStatusCancelled
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
			slog.ErrorContext(ctx, "failed to update cancelled refund job", "job_id", job.ID, "error", updateErr)
		}
		return fmt.Errorf("refund job cancelled: payment %s not refundable", job.PaymentID)
	}

	slog.InfoContext(ctx, "processing refund",
		"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID,
		"gateway_txn_id", p.GatewayTxnID, "amount", p.Amount)

	resp, gwErr := s.gateway.Refund(ctx, gateway.RefundRequest{
		// Key on the payment id: a payment is refunded once, so a job re-claimed
		// after a crash between this call and the commit reuses the same key and
		// the gateway dedupes it instead of refunding twice.
		IdempotencyKey: p.ID.String(),
		TransactionID:  p.GatewayTxnID,
		Amount:         p.Amount,
		Reason:         "auto-refund",
	})

	job.Attempts++

	if gwErr != nil {
		slog.ErrorContext(ctx, "refund failed",
			"job_id", job.ID, "order_id", job.OrderID, "attempt", job.Attempts, "error", gwErr)
		job.LastError = gwErr.Error()

		if job.Attempts >= job.MaxAttempts {
			job.Status = JobStatusFailed
		} else {
			job.Status = JobStatusPending
			backoff := time.Duration(1<<min(max(job.Attempts, 0), 30)) * time.Second
			jitter := time.Duration(rand.N(int64(backoff / jitterDivisor))) //nolint:gosec // jitter doesn't need crypto randomness
			job.NextRetryAt = time.Now().Add(backoff + jitter)
		}
		job.LockedUntil = nil
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
			slog.ErrorContext(ctx, "failed to update refund job after failure", "job_id", job.ID, "error", updateErr)
		}
		return fmt.Errorf("refund failed: %w", gwErr)
	}

	slog.InfoContext(ctx, "refund completed",
		"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID,
		"refund_id", resp.RefundID)

	txErr := database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
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
			slog.ErrorContext(txCtx, "failed to update payment status to refunded", "payment_id", job.PaymentID, "error", statusErr)
		}
		if orderStatusErr := s.orders.MarkRefunded(txCtx, job.OrderID); orderStatusErr != nil {
			slog.ErrorContext(txCtx, "failed to update order status to refunded", "order_id", job.OrderID, "error", orderStatusErr)
		}

		items, listErr := s.orderItems.ListItemsByOrderID(txCtx, job.OrderID)
		if listErr != nil {
			return listErr
		}
		switch {
		case orderSnap.Dispatched:
			// The goods already left the warehouse, so refund the money but do NOT
			// restock — adding shipped units back to sellable stock would oversell.
			slog.InfoContext(txCtx, "refund: skipping inventory restock for dispatched order",
				"order_id", job.OrderID, "order_status", orderSnap.Status)
		case len(items) > 0 && !orderSnap.StockReversed:
			// Inventory owns release-vs-restock; we pass the order's persisted fact.
			if restoreErr := s.inventoryRestorer.Restore(txCtx, toInventoryChanges(items), orderSnap.StockDeducted); restoreErr != nil {
				slog.ErrorContext(txCtx, "failed to restore inventory on refund",
					"order_id", job.OrderID, "error", restoreErr)
			}
		}

		if s.couponReleaser != nil && orderSnap.CouponCode != "" {
			if releaseErr := s.couponReleaser.Release(txCtx, job.OrderID); releaseErr != nil {
				slog.WarnContext(txCtx, "failed to release coupon on refund", "error", releaseErr)
			}
		}

		if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
			slog.ErrorContext(txCtx, "failed to mark refund job completed", "job_id", job.ID, "error", markErr)
		}
		return nil
	})

	if txErr != nil {
		return fmt.Errorf("refund finalization failed: %w", txErr)
	}
	return nil
}

func (s *Service) HandleWebhook(ctx context.Context, payload map[string]any) error { //nolint:gocognit // resolves the payment then dispatches success/failed/cancelled/expired event branches
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
					slog.ErrorContext(ctx, "webhook: failed to get payment by id", "payment_id", pid, "error", getErr)
				} else {
					p = found
				}
			}
		}
	}

	if p == nil && txnID != "" {
		found, getErr := s.repo.GetByGatewayTxnID(ctx, txnID)
		if getErr != nil {
			if !errors.Is(getErr, core.ErrNotFound) {
				slog.ErrorContext(ctx, "webhook: failed to get payment by gateway txn id", "txn_id", txnID, "error", getErr)
			}
		} else {
			p = found
		}
	}

	if p == nil {
		slog.ErrorContext(ctx, "webhook: unknown payment_id", "payload_event", event)
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
			if errors.Is(err, core.ErrAlreadyFinalized) {
				break
			}
			// The gateway has already captured funds, so a finalization failure
			// (e.g. inventory deduction failed) must not just 5xx and leave money
			// captured with the order unpaid forever. Compensate the same way the
			// worker charge path does: flag the order fulfillment_failed and enqueue
			// a refund. Ack the webhook so the gateway stops retrying into a failure
			// we've already handled.
			slog.ErrorContext(ctx, "webhook finalization failed, running compensating refund",
				"payment_id", p.ID, "order_id", p.OrderID, "error", err)
			s.runCompensatingRefund(ctx, job)
			return nil
		}
		if err := s.repo.MarkJobCompletedByPaymentID(ctx, p.ID, ActionCharge); err != nil {
			slog.ErrorContext(ctx, "webhook: failed to mark job completed by payment id", "payment_id", p.ID, "error", err)
		}

	case "failed", "cancelled", "expired":
		if err := s.repo.UpdateStatus(ctx, p.ID, StatusCancelled,
			[]Status{StatusPending, StatusProcessing}); err != nil {
			slog.ErrorContext(ctx, "webhook: failed to update payment status to cancelled", "payment_id", p.ID, "error", err)
		}
		if err := s.repo.ClearPaymentURL(ctx, p.ID); err != nil {
			slog.ErrorContext(ctx, "webhook: failed to clear payment url", "payment_id", p.ID, "error", err)
		}
		if err := s.repo.CancelJobsByOrderID(ctx, p.OrderID); err != nil {
			slog.ErrorContext(ctx, "webhook: failed to cancel jobs", "order_id", p.OrderID, "error", err)
		}
		// Cancel the order and release its reserved stock now rather than leaving it
		// reserved until the expiry sweep (which can't touch a payment_processing
		// order at all). ErrBadRequest means the order is no longer cancellable
		// (e.g. a concurrent charge already paid it) — leave it for that flow.
		if err := s.orders.CancelUnpaid(ctx, p.OrderID); err != nil && !errors.Is(err, core.ErrBadRequest) {
			slog.ErrorContext(ctx, "webhook: failed to cancel order after payment failure", "order_id", p.OrderID, "error", err)
		}
		slog.InfoContext(ctx, "webhook payment failed",
			"payment_id", p.ID, "gateway_event", event)
	}

	return nil
}

func (s *Service) CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error {
	return s.repo.CancelJobsByOrderID(ctx, orderID)
}

func (s *Service) Refund(ctx context.Context, paymentID uuid.UUID) error {
	p, err := s.repo.GetByID(ctx, paymentID)
	if err != nil {
		return err
	}

	if p.Status != StatusSuccess && p.Status != StatusRequiresReview {
		return fmt.Errorf("%w: payment is not refundable", core.ErrBadRequest)
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
