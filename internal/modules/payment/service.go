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

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway"
	gatewaymidtrans "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway/midtrans"
	gatewaymock "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway/mock"
	gatewaystripe "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway/stripe"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

type Service struct {
	repo    Repository
	tx      database.TxRunner
	gateway Gateway
	queue   RefundQueue
	logger  *slog.Logger

	orders    Orders
	inventory Inventory
	coupon    CouponReleaser

	webhookSecret string
}

func New(
	repo Repository,
	tx database.TxRunner,
	cfg Config,
	logger *slog.Logger,
	queue RefundQueue,
	orders Orders,
	inventory Inventory,
	coupons CouponReleaser,
) *Service {
	return &Service{
		repo:          repo,
		tx:            tx,
		gateway:       newGateway(cfg),
		queue:         queue,
		logger:        logger,
		orders:        orders,
		inventory:     inventory,
		coupon:        coupons,
		webhookSecret: cfg.WebhookSecret,
	}
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
	if err != nil && !errors.Is(err, errs.ErrNotFound) {
		return ChargeResult{}, err
	}

	var p *domain.Payment
	if !errors.Is(err, errs.ErrNotFound) {
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
		if finalizeErr := s.FinalizeSuccess(
			ctx,
			p.ID,
			req.OrderID,
		); finalizeErr != nil &&
			!errors.Is(finalizeErr, apperror.ErrAlreadyFinalized) {
			s.logger.ErrorContext(
				ctx,
				"synchronous charge succeeded but finalization failed, running compensating refund",
				slog.String("payment_id", p.ID.String()),
				slog.String("order_id", req.OrderID.String()),
				slog.String("error", finalizeErr.Error()),
			)
			s.CompensateRefund(ctx, p.ID, req.OrderID)
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
		return result, fmt.Errorf("%w: payment was declined", errs.ErrBadRequest)
	}

	return result, nil
}

//nolint:gocognit // single finalize CAS with idempotent already-finalized and late-charge-on-terminal-order branches; funlen counts golines' wrapping, not added logic
func (s *Service) FinalizeSuccess(ctx context.Context, paymentID, orderID uuid.UUID) error {
	return s.tx.Run(ctx, func(txCtx context.Context) error {
		orderSnap, err := s.orders.Snapshot(txCtx, orderID)
		if err != nil {
			return fmt.Errorf("getting order for verification: %w", err)
		}

		p, err := s.repo.GetByID(txCtx, paymentID)
		if err != nil {
			return fmt.Errorf("getting payment for verification: %w", err)
		}

		if !p.Amount.Equal(orderSnap.Total) {
			return apperror.ErrAmountMismatch
		}

		paymentErr := s.repo.MarkPaid(
			txCtx,
			paymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			},
		)

		orderErr := s.orders.MarkPaid(txCtx, orderID)

		if paymentErr != nil && orderErr != nil {
			s.logger.InfoContext(txCtx, "job completed: already finalized by external actor (webhook)",
				slog.String("order_id", orderID.String()), slog.String("payment_id", paymentID.String()))
			return apperror.ErrAlreadyFinalized
		}

		if orderErr != nil {
			s.logger.ErrorContext(txCtx, "late payment success on terminal order, auto-refund enqueued",
				slog.String("order_id", orderID.String()), slog.String("payment_id", paymentID.String()),
				slog.String("order_status", orderSnap.Status))
			if statusErr := s.repo.UpdateStatus(txCtx, paymentID, domain.StatusRequiresReview,
				[]domain.Status{domain.StatusSuccess}); statusErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to update payment status to requires_review",
					slog.String("payment_id", paymentID.String()),
					slog.String("error", statusErr.Error()),
				)
			}
			if orderStatusErr := s.orders.MarkFulfillmentFailedAfterCharge(
				txCtx, orderID,
			); orderStatusErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to update order status to fulfillment_failed",
					slog.String("order_id", orderID.String()),
					slog.String("error", orderStatusErr.Error()),
				)
			}

			if createErr := s.queue.EnqueueRefund(txCtx, paymentID, orderID); createErr != nil {
				s.logger.ErrorContext(
					txCtx,
					"failed to create refund job",
					slog.String("order_id", orderID.String()),
					slog.String("error", createErr.Error()),
				)
			}
			return nil
		}

		items, err := s.orders.ListItemQuantities(txCtx, orderID)
		if err != nil {
			return fmt.Errorf("listing order items: %w", err)
		}

		if err := s.inventory.Deduct(txCtx, items); err != nil {
			return fmt.Errorf("deducting inventory: %w", err)
		}

		return nil
	})
}

func (s *Service) CompensateRefund(ctx context.Context, paymentID, orderID uuid.UUID) {
	txErr := s.tx.Run(ctx, func(txCtx context.Context) error {
		if statusErr := s.repo.UpdateStatus(txCtx, paymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing, domain.StatusSuccess}); statusErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update payment status in compensating refund",
				slog.String("payment_id", paymentID.String()),
				slog.String("error", statusErr.Error()),
			)
		}
		if orderErr := s.orders.MarkFulfillmentFailedCompensating(txCtx, orderID); orderErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update order status in compensating refund",
				slog.String("order_id", orderID.String()),
				slog.String("error", orderErr.Error()),
			)
		}

		return s.queue.EnqueueRefund(txCtx, paymentID, orderID)
	})
	if txErr != nil {
		s.logger.ErrorContext(ctx, "compensating refund failed",
			slog.String("order_id", orderID.String()), slog.String("error", txErr.Error()))
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
		return fmt.Errorf("%w: payment is not refundable", errs.ErrBadRequest)
	}

	return s.queue.EnqueueRefund(ctx, paymentID, p.OrderID)
}

//nolint:gocognit // resolves the payment then dispatches success/failed/cancelled/expired event branches
func (s *Service) HandleWebhook(ctx context.Context, payload []byte, signature string) error {
	if s.webhookSecret != "" && !verifySignature(s.webhookSecret, payload, signature) {
		s.logger.WarnContext(ctx, "webhook: invalid or missing signature")
		return fmt.Errorf("%w: invalid webhook signature", errs.ErrUnauthorized)
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
			if !errors.Is(getErr, errs.ErrNotFound) {
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
		if err := s.FinalizeSuccess(ctx, p.ID, p.OrderID); err != nil {
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
			s.CompensateRefund(ctx, p.ID, p.OrderID)
			return nil
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
		if err := s.orders.CancelUnpaid(ctx, p.OrderID); err != nil && !errors.Is(err, errs.ErrBadRequest) {
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

//nolint:gocognit // not-refundable guard, gateway call, and the finalize transaction's dispatched/restock/coupon branches
func (s *Service) SettleRefund(ctx context.Context, paymentID, orderID uuid.UUID) error {
	p, err := s.repo.GetByID(ctx, paymentID)
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
			slog.String("payment_id", paymentID.String()))
		return fmt.Errorf("payment %s not refundable: %w", paymentID, ErrNotRefundable)
	}

	s.logger.InfoContext(
		ctx,
		"processing refund",
		slog.String("order_id", orderID.String()),
		slog.String("payment_id", paymentID.String()),
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
	if gwErr != nil {
		s.logger.ErrorContext(
			ctx,
			"refund failed",
			slog.String("order_id", orderID.String()),
			slog.String("error", gwErr.Error()),
		)
		return fmt.Errorf("refund failed: %w", gwErr)
	}

	s.logger.InfoContext(ctx, "refund completed",
		slog.String("order_id", orderID.String()), slog.String("payment_id", paymentID.String()),
		slog.String("refund_id", resp.RefundID))

	txErr := s.tx.Run(ctx, func(txCtx context.Context) error {
		orderSnap, snapErr := s.orders.Snapshot(txCtx, orderID)
		if snapErr != nil {
			return fmt.Errorf("getting order for refund: %w", snapErr)
		}

		if statusErr := s.repo.UpdateStatus(txCtx, paymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}); statusErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update payment status to refunded",
				slog.String("payment_id", paymentID.String()),
				slog.String("error", statusErr.Error()),
			)
		}
		if orderStatusErr := s.orders.MarkRefunded(txCtx, orderID); orderStatusErr != nil {
			s.logger.ErrorContext(
				txCtx,
				"failed to update order status to refunded",
				slog.String("order_id", orderID.String()),
				slog.String("error", orderStatusErr.Error()),
			)
		}

		items, listErr := s.orders.ListItemQuantities(txCtx, orderID)
		if listErr != nil {
			return listErr
		}
		switch {
		case orderSnap.Dispatched:
			s.logger.InfoContext(txCtx, "refund: skipping inventory restock for dispatched order",
				slog.String("order_id", orderID.String()), slog.String("order_status", orderSnap.Status))
		case len(items) > 0 && !orderSnap.StockReversed:
			if restoreErr := s.inventory.Restore(
				txCtx,
				items,
				stockStateFor(orderSnap.StockDeducted),
			); restoreErr != nil {
				s.logger.ErrorContext(txCtx, "failed to restore inventory on refund",
					slog.String("order_id", orderID.String()), slog.String("error", restoreErr.Error()))
			}
		}

		if s.coupon != nil && orderSnap.CouponCode != "" {
			if releaseErr := s.coupon.Release(txCtx, orderID); releaseErr != nil {
				s.logger.WarnContext(
					txCtx,
					"failed to release coupon on refund",
					slog.String("error", releaseErr.Error()),
				)
			}
		}

		return nil
	})

	if txErr != nil {
		return fmt.Errorf("refund finalization failed: %w", txErr)
	}
	return nil
}

func (s *Service) CancelPendingByOrderID(ctx context.Context, orderID uuid.UUID) error {
	if err := s.queue.CancelPendingForOrder(ctx, orderID); err != nil {
		return fmt.Errorf("cancelling payment jobs for order %s: %w", orderID, err)
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
