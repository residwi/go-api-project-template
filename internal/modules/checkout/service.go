package checkout

import (
	"context"
	"errors"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
)

type PlaceOrderInput struct {
	Order           orderdomain.NewOrder
	PaymentMethodID string
	IdempotencyKey  string
}

type Service struct {
	orders   Orders
	payments Payments
	logger   *slog.Logger
}

func New(orders Orders, payments Payments, logger *slog.Logger) *Service {
	return &Service{orders: orders, payments: payments, logger: logger}
}

func (s *Service) PlaceOrder(
	ctx context.Context,
	userID uuid.UUID,
	in PlaceOrderInput,
) (*orderdomain.Order, error) {
	order, created, err := s.orders.Place(ctx, userID, in.Order, in.IdempotencyKey)
	if err != nil {
		return nil, err
	}

	// A replay of an idempotency key returns the stored order, which was already
	// charged on the call that created it. Charging again bills the customer
	// twice and lands the order in fulfillment_failed, so the same successful
	// response is returned untouched.
	if created && order.Total.Amount > 0 {
		if _, payErr := s.payments.Charge(ctx, payment.ChargeRequest{
			OrderID:         order.ID,
			Amount:          order.Total,
			PaymentMethodID: in.PaymentMethodID,
		}); payErr != nil {
			s.logger.ErrorContext(ctx, "failed to initiate payment, order stays in awaiting_payment",
				slog.String("order_id", order.ID.String()), slog.String("error", payErr.Error()))
		}
	}

	return order, nil
}

func (s *Service) RetryPayment(
	ctx context.Context,
	userID, orderID uuid.UUID,
	paymentMethodID string,
) (payment.ChargeResult, error) {
	order, err := s.orders.Snapshot(ctx, orderID)
	if err != nil {
		return payment.ChargeResult{}, err
	}
	if order.UserID != userID {
		return payment.ChargeResult{}, apperror.ErrNotFound
	}
	if claimErr := s.orders.BeginPaymentAttempt(ctx, orderID); claimErr != nil {
		if errors.Is(claimErr, apperror.ErrConflict) {
			return payment.ChargeResult{}, apperror.ErrOrderNotPayable
		}
		return payment.ChargeResult{}, claimErr
	}

	result, err := s.payments.Charge(ctx, payment.ChargeRequest{
		OrderID:         order.ID,
		Amount:          order.Total,
		PaymentMethodID: paymentMethodID,
	})
	if err != nil {
		if releaseErr := s.orders.MarkAwaitingPayment(ctx, orderID); releaseErr != nil {
			s.logger.ErrorContext(ctx, "failed to release the payment attempt claim",
				slog.String("order_id", orderID.String()), slog.String("error", releaseErr.Error()))
		}
		return result, err
	}

	return result, nil
}

func (s *Service) CancelOrder(ctx context.Context, userID, orderID uuid.UUID) error {
	if err := s.orders.CancelByUser(ctx, userID, orderID); err != nil {
		return err
	}

	if err := s.payments.CancelPendingByOrderID(ctx, orderID); err != nil {
		s.logger.WarnContext(ctx, "failed to cancel payment jobs",
			slog.String("order_id", orderID.String()), slog.String("error", err.Error()))
	}

	return nil
}
