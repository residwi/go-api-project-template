package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service   *payment.Service
	validator *validator.Validator
}

// adminPaymentResponse is admin-only -- there is no public payment response
// to collide with, since a shopper never sees a payment object directly
// (the "pay" flow lives on order's wire, not payment's). GatewayResponse is
// the one field dropped for cause: it carries raw gateway payloads that may
// include PII or card metadata. Everything else on Payment is operator-
// facing account data, including OrderID -- an admin needs it to correlate
// a payment back to the order it belongs to.
//
// Amount and Currency stay two keys even though payment.Payment now holds one
// money.Money: this endpoint has always emitted both, so the value is flattened
// field-by-field here rather than by a MarshalJSON that could not know which
// endpoints want a currency key and which do not. See internal/money/doc.go.
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

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
}

func (h *adminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	p, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminPaymentResponse(p))
}

// refundResponse replaces the pre-refactor inline map[string]string literal
// with a named type -- same wire shape, now typed. Refund has no request
// params.go counterpart: payment.Service.Refund already takes a plain
// uuid.UUID, not a request struct (a partial-amount/reasoned refund isn't
// implemented today, so there is nothing to bind from a body).
type refundResponse struct {
	Status string `json:"status"`
}

func (h *adminHandler) Refund(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.Refund(r.Context(), id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, refundResponse{Status: "refund_enqueued"})
}
