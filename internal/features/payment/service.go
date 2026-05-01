package payment

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	gateway "github.com/residwi/go-api-project-template/internal/platform/payment"
)

const (
	inventoryActionRelease = "release"
	inventoryActionRestock = "restock"
	jitterDivisor          = 2

	// order status strings for cross-domain calls (payment package defines its own
	// OrderUpdater interface with string params to avoid importing the order package)
	orderStatusAwaitingPayment   = "awaiting_payment"
	orderStatusPaymentProcessing = "payment_processing"
	orderStatusPaid              = "paid"
	orderStatusCancelled         = "cancelled"
	orderStatusExpired           = "expired"
	orderStatusFulfillmentFailed = "fulfillment_failed"
	orderStatusDelivered         = "delivered"
	orderStatusRefunded          = "refunded"
)

type OrderUpdater interface {
	UpdateStatus(ctx context.Context, orderID uuid.UUID, fromStatuses []string, toStatus string) error
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
}

type OrderGetter interface {
	GetByID(ctx context.Context, orderID uuid.UUID) (OrderSnapshot, error)
}

type InventoryDeductor interface {
	Deduct(ctx context.Context, productID uuid.UUID, qty int) error
}

type InventoryReleaser interface {
	Release(ctx context.Context, productID uuid.UUID, qty int) error
}

type InventoryRestocker interface {
	Restock(ctx context.Context, productID uuid.UUID, qty int) error
}

type CouponReleaser interface {
	Release(ctx context.Context, orderID uuid.UUID) error
}

type Service struct {
	repo               Repository
	pool               *pgxpool.Pool
	gateway            gateway.Gateway
	orders             OrderUpdater
	orderGet           OrderGetter
	orderItems         OrderItemsGetter
	inventory          InventoryDeductor
	inventoryReleaser  InventoryReleaser
	inventoryRestocker InventoryRestocker
	couponReleaser     CouponReleaser
}

func NewService(
	repo Repository,
	pool *pgxpool.Pool,
	gw gateway.Gateway,
	orders OrderUpdater,
	orderGet OrderGetter,
	orderItems OrderItemsGetter,
	inventory InventoryDeductor,
	inventoryReleaser InventoryReleaser,
	inventoryRestocker InventoryRestocker,
	couponReleaser CouponReleaser,
) *Service {
	return &Service{
		repo:               repo,
		pool:               pool,
		gateway:            gw,
		orders:             orders,
		orderGet:           orderGet,
		orderItems:         orderItems,
		inventory:          inventory,
		inventoryReleaser:  inventoryReleaser,
		inventoryRestocker: inventoryRestocker,
		couponReleaser:     couponReleaser,
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
	case string(StatusPending):
		if resp.PaymentURL != "" {
			if err := s.repo.UpdatePaymentURL(ctx, p.ID, resp.PaymentURL); err != nil {
				slog.ErrorContext(ctx, "failed to update payment url", "error", err)
			}
			result.PaymentURL = resp.PaymentURL
		}
	}

	return result, nil
}

func (s *Service) ProcessJob(ctx context.Context, job Job) bool {
	switch job.Action {
	case ActionCharge:
		return s.processChargeJob(ctx, job)
	case ActionRefund:
		return s.processRefundJob(ctx, job)
	default:
		slog.ErrorContext(ctx, "unknown job action", "job_id", job.ID, "action", job.Action)
		return false
	}
}

func (s *Service) processChargeJob(ctx context.Context, job Job) bool {
	err := s.orders.UpdateStatus(ctx, job.OrderID,
		[]string{orderStatusAwaitingPayment, orderStatusPaymentProcessing}, orderStatusPaymentProcessing)
	if err != nil {
		slog.WarnContext(ctx, "charge job cancelled: order not in expected state",
			"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID)
		job.Status = JobStatusCancelled
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
			slog.ErrorContext(ctx, "failed to update cancelled job", "job_id", job.ID, "error", updateErr)
		}
		return false
	}

	p, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get payment for job", "job_id", job.ID, "error", err)
		return false
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
		return false
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
				return true
			}
			slog.ErrorContext(ctx, "finalization failed, running compensating flow",
				"job_id", job.ID, "order_id", job.OrderID, "error", finalizeErr)
			s.runCompensatingRefund(ctx, job)
		}
		return true

	default:
		slog.WarnContext(ctx, "charge returned non-success",
			"job_id", job.ID, "order_id", job.OrderID, "status", resp.Status,
			"attempt", job.Attempts)
		s.handleChargeFailure(ctx, &job, fmt.Sprintf("gateway returned status: %s", resp.Status))
		return false
	}
}

func (s *Service) handleChargeFailure(ctx context.Context, job *Job, lastError string) {
	job.LastError = lastError

	if err := s.orders.UpdateStatus(ctx, job.OrderID,
		[]string{orderStatusPaymentProcessing}, orderStatusAwaitingPayment); err != nil {
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

//nolint:gocognit
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

		orderErr := s.orders.UpdateStatus(txCtx, job.OrderID,
			[]string{orderStatusPaymentProcessing, orderStatusAwaitingPayment}, orderStatusPaid)

		if paymentErr != nil && orderErr != nil {
			slog.InfoContext(txCtx, "job completed: already finalized by external actor (webhook)",
				"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID)
			if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
				slog.ErrorContext(txCtx, "failed to mark job completed", "job_id", job.ID, "error", markErr)
			}
			return core.ErrAlreadyFinalized
		}

		if orderErr != nil { //nolint:nestif // complex error recovery logic
			slog.ErrorContext(txCtx, "late payment success on terminal order, auto-refund enqueued",
				"order_id", job.OrderID, "payment_id", job.PaymentID,
				"order_status", orderSnap.Status)
			if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRequiresReview,
				[]Status{StatusSuccess}); statusErr != nil {
				slog.ErrorContext(txCtx, "failed to update payment status to requires_review", "payment_id", job.PaymentID, "error", statusErr)
			}
			if orderStatusErr := s.orders.UpdateStatus(txCtx, job.OrderID,
				[]string{orderStatusCancelled, orderStatusExpired, orderStatusPaid}, orderStatusFulfillmentFailed); orderStatusErr != nil {
				slog.ErrorContext(txCtx, "failed to update order status to fulfillment_failed", "order_id", job.OrderID, "error", orderStatusErr)
			}

			inventoryAction := inventoryActionRelease
			if orderSnap.Status == orderStatusPaid || orderSnap.Status == orderStatusDelivered {
				inventoryAction = inventoryActionRestock
			}
			refundJob := &Job{
				PaymentID:       job.PaymentID,
				OrderID:         job.OrderID,
				Action:          ActionRefund,
				Status:          JobStatusPending,
				NextRetryAt:     time.Now(),
				InventoryAction: inventoryAction,
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

		sort.Slice(items, func(i, j int) bool {
			return items[i].ProductID.String() < items[j].ProductID.String()
		})

		for _, item := range items {
			if err := s.inventory.Deduct(txCtx, item.ProductID, item.Quantity); err != nil {
				return fmt.Errorf("deducting inventory: %w", err)
			}
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
		if orderErr := s.orders.UpdateStatus(txCtx, job.OrderID,
			[]string{orderStatusPaymentProcessing, orderStatusAwaitingPayment, orderStatusCancelled, orderStatusExpired, orderStatusPaid},
			orderStatusFulfillmentFailed); orderErr != nil {
			slog.ErrorContext(txCtx, "failed to update order status in compensating refund", "order_id", job.OrderID, "error", orderErr)
		}

		refundJob := &Job{
			PaymentID:       job.PaymentID,
			OrderID:         job.OrderID,
			Action:          ActionRefund,
			Status:          JobStatusPending,
			NextRetryAt:     time.Now(),
			InventoryAction: inventoryActionRelease,
		}
		return s.repo.CreateJob(txCtx, refundJob)
	})
	if txErr != nil {
		slog.ErrorContext(ctx, "compensating refund failed",
			"job_id", job.ID, "order_id", job.OrderID, "error", txErr)
	}
}

//nolint:gocognit
func (s *Service) processRefundJob(ctx context.Context, job Job) bool {
	p, err := s.repo.GetByID(ctx, job.PaymentID)
	if err != nil {
		slog.ErrorContext(ctx, "failed to get payment for refund", "job_id", job.ID, "error", err)
		return false
	}

	if p.Status != StatusSuccess && p.Status != StatusRequiresReview {
		slog.WarnContext(ctx, "refund job cancelled: payment not refundable",
			"job_id", job.ID, "payment_status", p.Status)
		job.Status = JobStatusCancelled
		if updateErr := s.repo.UpdateJob(ctx, &job); updateErr != nil {
			slog.ErrorContext(ctx, "failed to update cancelled refund job", "job_id", job.ID, "error", updateErr)
		}
		return false
	}

	slog.InfoContext(ctx, "processing refund",
		"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID,
		"gateway_txn_id", p.GatewayTxnID, "amount", p.Amount)

	resp, gwErr := s.gateway.Refund(ctx, gateway.RefundRequest{
		TransactionID: p.GatewayTxnID,
		Amount:        p.Amount,
		Reason:        "auto-refund",
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
		return false
	}

	slog.InfoContext(ctx, "refund completed",
		"job_id", job.ID, "order_id", job.OrderID, "payment_id", job.PaymentID,
		"refund_id", resp.RefundID)

	txErr := database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		if statusErr := s.repo.UpdateStatus(txCtx, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}); statusErr != nil {
			slog.ErrorContext(txCtx, "failed to update payment status to refunded", "payment_id", job.PaymentID, "error", statusErr)
		}
		if orderStatusErr := s.orders.UpdateStatus(txCtx, job.OrderID,
			[]string{orderStatusFulfillmentFailed, orderStatusPaid, orderStatusDelivered}, orderStatusRefunded); orderStatusErr != nil {
			slog.ErrorContext(txCtx, "failed to update order status to refunded", "order_id", job.OrderID, "error", orderStatusErr)
		}

		items, listErr := s.orderItems.ListItemsByOrderID(txCtx, job.OrderID)
		if listErr != nil {
			return listErr
		}
		sort.Slice(items, func(i, j int) bool {
			return items[i].ProductID.String() < items[j].ProductID.String()
		})
		for _, item := range items {
			switch job.InventoryAction {
			case inventoryActionRelease:
				if releaseErr := s.inventoryReleaser.Release(txCtx, item.ProductID, item.Quantity); releaseErr != nil {
					slog.ErrorContext(txCtx, "failed to release inventory on refund",
						"product_id", item.ProductID, "error", releaseErr)
				}
			case inventoryActionRestock:
				if restockErr := s.inventoryRestocker.Restock(txCtx, item.ProductID, item.Quantity); restockErr != nil {
					slog.ErrorContext(txCtx, "failed to restock inventory on refund",
						"product_id", item.ProductID, "error", restockErr)
				}
			}
		}

		if s.couponReleaser != nil {
			orderSnap, getErr := s.orderGet.GetByID(txCtx, job.OrderID)
			if getErr == nil && orderSnap.CouponCode != "" {
				if releaseErr := s.couponReleaser.Release(txCtx, job.OrderID); releaseErr != nil {
					slog.WarnContext(txCtx, "failed to release coupon on refund", "error", releaseErr)
				}
			}
		}

		if markErr := s.repo.MarkJobCompleted(txCtx, job.ID); markErr != nil {
			slog.ErrorContext(txCtx, "failed to mark refund job completed", "job_id", job.ID, "error", markErr)
		}
		return nil
	})

	return txErr == nil
}

func (s *Service) HandleWebhook(ctx context.Context, payload map[string]any) error { //nolint:gocognit
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

	if p.Status == StatusSuccess || p.Status == StatusRefunded {
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
			if !errors.Is(err, core.ErrAlreadyFinalized) {
				return err
			}
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

	orderSnap, err := s.orderGet.GetByID(ctx, p.OrderID)
	if err != nil {
		return err
	}

	inventoryAction := inventoryActionRelease
	if orderSnap.Status == orderStatusPaid || orderSnap.Status == orderStatusDelivered {
		inventoryAction = inventoryActionRestock
	}

	job := &Job{
		PaymentID:       paymentID,
		OrderID:         p.OrderID,
		Action:          ActionRefund,
		Status:          JobStatusPending,
		NextRetryAt:     time.Now(),
		InventoryAction: inventoryAction,
	}

	return s.repo.CreateJob(ctx, job)
}
