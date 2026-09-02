package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/payment/domain"
)

type adminPaymentResponse struct {
	ID              uuid.UUID     `json:"id"`
	OrderID         uuid.UUID     `json:"order_id"`
	Amount          int64         `json:"amount"`
	Currency        string        `json:"currency"`
	Status          domain.Status `json:"status"`
	Method          string        `json:"method,omitempty"`
	PaymentMethodID string        `json:"payment_method_id,omitempty"`
	PaymentURL      string        `json:"payment_url,omitempty"`
	GatewayTxnID    string        `json:"gateway_txn_id,omitempty"`
	PaidAt          *time.Time    `json:"paid_at,omitempty"`
	CreatedAt       time.Time     `json:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"`
}

func toAdminPaymentResponse(p *domain.Payment) adminPaymentResponse {
	return adminPaymentResponse{
		ID:              p.ID,
		OrderID:         p.OrderID,
		Amount:          p.Amount.Amount,
		Currency:        p.Amount.Currency,
		Status:          p.Status,
		Method:          p.Method,
		PaymentMethodID: p.PaymentMethodID,
		PaymentURL:      p.PaymentURL,
		GatewayTxnID:    p.GatewayTxnID,
		PaidAt:          p.PaidAt,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

type refundResponse struct {
	Status string `json:"status"`
}
