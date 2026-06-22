package payment

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
	// IdempotencyKey lets the gateway dedupe a refund that is retried after a
	// crash between the gateway call and the local commit, so the customer is not
	// refunded twice. Stable per payment (one refund per payment).
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
