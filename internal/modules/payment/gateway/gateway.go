// Package gateway is payment's outbound adapter family: the Gateway port plus
// the wire DTOs of the external payment gateway's protocol. It is not a
// slice -- charge and refund both depend on Gateway, and module.go picks one
// implementation from Config.Gateway to hand to both.
package gateway

import "context"

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
	// Stable per payment, so a refund retried after a crash between the gateway call
	// and the local commit is deduped rather than paid twice.
	IdempotencyKey string `json:"idempotency_key"`
	TransactionID  string `json:"transaction_id"`
	Amount         int64  `json:"amount"`
	Reason         string `json:"reason"`
}

type RefundResponse struct {
	RefundID string `json:"refund_id"`
	Status   string `json:"status"`
}

type Gateway interface {
	Charge(ctx context.Context, req ChargeRequest) (ChargeResponse, error)
	Refund(ctx context.Context, req RefundRequest) (RefundResponse, error)
}
