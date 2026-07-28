package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/payment"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// adminPaymentResponse is admin-only -- there is no public payment response
// to collide with, since a shopper never sees a payment object directly
// (the "pay" flow lives on order's wire, not payment's). GatewayResponse is
// the one field dropped for cause: it carries raw gateway payloads that may
// include PII or card metadata. Everything else on Payment is operator-
// facing account data, including OrderID -- an admin needs it to correlate
// a payment back to the order it belongs to.
type adminPaymentResponse struct {
	ID              uuid.UUID      `json:"id"`
	OrderID         uuid.UUID      `json:"order_id"`
	Amount          int64          `json:"amount"`
	Currency        string         `json:"currency"`
	Status          payment.Status `json:"status"`
	Method          string         `json:"method,omitempty"`
	PaymentMethodID string         `json:"payment_method_id,omitempty"`
	PaymentURL      string         `json:"payment_url,omitempty"`
	GatewayTxnID    string         `json:"gateway_txn_id,omitempty"`
	PaidAt          *time.Time     `json:"paid_at,omitempty"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

func toAdminPaymentResponse(p *payment.Payment) adminPaymentResponse {
	return adminPaymentResponse{
		ID:              p.ID,
		OrderID:         p.OrderID,
		Amount:          p.Amount,
		Currency:        p.Currency,
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

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := payment.AdminListParams{
		Page:     page.Page,
		PageSize: page.PageSize,
		Status:   r.URL.Query().Get("status"),
		OrderID:  r.URL.Query().Get("order_id"),
	}

	payments, total, err := h.service.ListAdmin(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]adminPaymentResponse, len(payments))
	for i, p := range payments {
		out[i] = toAdminPaymentResponse(&p)
	}

	response.Paginated(w, paging.NewOffsetPageResult(out, page, total))
}
