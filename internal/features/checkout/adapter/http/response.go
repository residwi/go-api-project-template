package http

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
)

type placedOrderResponse struct {
	ID       uuid.UUID `json:"id"`
	UserID   uuid.UUID `json:"user_id"`
	Status   string    `json:"status"`
	Total    int64     `json:"total"`
	Currency string    `json:"currency"`
}

type placeOrderResponse struct {
	Order placedOrderResponse `json:"order"`
}

func toPlaceOrderResponse(o *order.Snapshot) placeOrderResponse {
	return placeOrderResponse{Order: placedOrderResponse{
		ID:       o.ID,
		UserID:   o.UserID,
		Status:   o.Status,
		Total:    o.Total.Amount,
		Currency: o.Total.Currency,
	}}
}

type payResultResponse struct {
	PaymentID  uuid.UUID `json:"payment_id"`
	PaymentURL string    `json:"payment_url,omitempty"`
	Charged    bool      `json:"charged"`
}

func toPayResultResponse(r payment.ChargeResult) payResultResponse {
	return payResultResponse{
		PaymentID:  r.PaymentID,
		PaymentURL: r.PaymentURL,
		Charged:    r.Charged,
	}
}
