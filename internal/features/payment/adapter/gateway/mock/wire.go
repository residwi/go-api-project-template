package mock

import "github.com/residwi/go-api-project-template/internal/features/payment"

type ChargeRequest struct {
	IdempotencyKey  string            `json:"idempotency_key"`
	OrderID         string            `json:"order_id"`
	Amount          int64             `json:"amount"`
	Currency        string            `json:"currency"`
	Description     string            `json:"description"`
	PaymentMethodID string            `json:"payment_method_id,omitempty"`
	ReturnURL       string            `json:"return_url,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type ChargeResponse struct {
	TransactionID string `json:"transaction_id"`
	Status        string `json:"status"`
	PaymentURL    string `json:"payment_url,omitempty"`
}

type RefundRequest struct {
	IdempotencyKey string `json:"idempotency_key"`
	TransactionID  string `json:"transaction_id"`
	Amount         int64  `json:"amount"`
	Reason         string `json:"reason"`
}

type RefundResponse struct {
	RefundID string `json:"refund_id"`
	Status   string `json:"status"`
}

func chargeRequestFrom(req payment.GatewayChargeRequest) ChargeRequest {
	return ChargeRequest{
		IdempotencyKey:  req.IdempotencyKey,
		OrderID:         req.OrderID,
		Amount:          req.Amount,
		Currency:        req.Currency,
		Description:     req.Description,
		PaymentMethodID: req.PaymentMethodID,
		ReturnURL:       req.ReturnURL,
		Metadata:        req.Metadata,
	}
}

func (r ChargeResponse) toPayment() payment.GatewayChargeResponse {
	return payment.GatewayChargeResponse{
		TransactionID: r.TransactionID,
		Status:        r.Status,
		PaymentURL:    r.PaymentURL,
	}
}

func refundRequestFrom(req payment.GatewayRefundRequest) RefundRequest {
	return RefundRequest{
		IdempotencyKey: req.IdempotencyKey,
		TransactionID:  req.TransactionID,
		Amount:         req.Amount,
		Reason:         req.Reason,
	}
}

func (r RefundResponse) toPayment() payment.GatewayRefundResponse {
	return payment.GatewayRefundResponse{
		RefundID: r.RefundID,
		Status:   r.Status,
	}
}
