package checkout

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
)

type PlaceOrderInput struct {
	Order           orderdomain.NewOrder
	PaymentMethodID string
	IdempotencyKey  string
}

type Deps struct {
	Orders    OrderWriter
	Payments  PaymentCharger
	Snapshots OrderSnapshotReader
	Logger    *slog.Logger
}

type Service struct {
	orders    OrderWriter
	payments  PaymentCharger
	snapshots OrderSnapshotReader
	logger    *slog.Logger
}

func New(d Deps) *Service {
	return &Service{orders: d.Orders, payments: d.Payments, snapshots: d.Snapshots, logger: d.Logger}
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
		if _, payErr := s.payments.Charge(ctx, paymentcontract.ChargeRequest{
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
) (paymentcontract.ChargeResult, error) {
	order, err := s.snapshots.GetSnapshot(ctx, orderID)
	if err != nil {
		return paymentcontract.ChargeResult{}, err
	}
	if order.UserID != userID {
		return paymentcontract.ChargeResult{}, apperror.ErrNotFound
	}
	if order.Status != string(orderdomain.StatusAwaitingPayment) {
		return paymentcontract.ChargeResult{}, apperror.ErrOrderNotPayable
	}

	return s.payments.Charge(ctx, paymentcontract.ChargeRequest{
		OrderID:         order.ID,
		Amount:          order.Total,
		PaymentMethodID: paymentMethodID,
	})
}
