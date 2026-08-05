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

// Admin-only: a shopper never sees a payment object, the "pay" flow lives on
// order's wire. GatewayResponse is the one field dropped for cause -- raw gateway
// payloads that may carry PII or card metadata. Everything else is operator-facing
// account data, OrderID included.
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
		OffsetPage: page,
		Status:     r.URL.Query().Get("status"),
		OrderID:    r.URL.Query().Get("order_id"),
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
