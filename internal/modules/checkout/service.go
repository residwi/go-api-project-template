package checkout

import (
	"context"
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

type Deps struct {
	Orders      OrderWriter
	Payments    PaymentCharger
	Snapshots   OrderSnapshotReader
	Cancels     OrderCanceller
	PaymentJobs PaymentJobCanceller
	Logger      *slog.Logger
}

type Service struct {
	orders      OrderWriter
	payments    PaymentCharger
	snapshots   OrderSnapshotReader
	cancels     OrderCanceller
	paymentJobs PaymentJobCanceller
	logger      *slog.Logger
}

func New(d Deps) *Service {
	return &Service{
		orders:      d.Orders,
		payments:    d.Payments,
		snapshots:   d.Snapshots,
		cancels:     d.Cancels,
		paymentJobs: d.PaymentJobs,
		logger:      d.Logger,
	}
}

func (s *Service) PlaceOrder(
	ctx context.Context,
	userID uuid.UUID,
	in PlaceOrderInput,
) (*orderdomain.Order, error) {
	order, err := s.orders.Place(ctx, userID, in.Order, in.IdempotencyKey)
	if err != nil {
		return nil, err
	}

	if order.Total.Amount > 0 {
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
	order, err := s.snapshots.Snapshot(ctx, orderID)
	if err != nil {
		return payment.ChargeResult{}, err
	}
	if order.UserID != userID {
		return payment.ChargeResult{}, apperror.ErrNotFound
	}
	if order.Status != string(orderdomain.StatusAwaitingPayment) {
		return payment.ChargeResult{}, apperror.ErrOrderNotPayable
	}

	return s.payments.Charge(ctx, payment.ChargeRequest{
		OrderID:         order.ID,
		Amount:          order.Total,
		PaymentMethodID: paymentMethodID,
	})
}

func (s *Service) CancelOrder(ctx context.Context, userID, orderID uuid.UUID) error {
	if err := s.cancels.CancelByUser(ctx, userID, orderID); err != nil {
		return err
	}

	// Best effort: the order is already cancelled, and a job that fires against
	// a cancelled order is rejected by its own status guard.
	if err := s.paymentJobs.CancelPendingByOrderID(ctx, orderID); err != nil {
		s.logger.WarnContext(ctx, "failed to cancel payment jobs",
			slog.String("order_id", orderID.String()), slog.String("error", err.Error()))
	}

	return nil
}
