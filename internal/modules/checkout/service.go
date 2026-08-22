package checkout

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
)

type PlaceOrderInput struct {
	Order           orderdomain.NewOrder
	PaymentMethodID string
	IdempotencyKey  string
}

type Deps struct {
	Orders   OrderWriter
	Payments PaymentCharger
	Logger   *slog.Logger
}

type Service struct {
	orders   OrderWriter
	payments PaymentCharger
	logger   *slog.Logger
}

func New(d Deps) *Service {
	return &Service{orders: d.Orders, payments: d.Payments, logger: d.Logger}
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
