package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/server/response"
)

// PaymentManager merges query's PaymentReader and refund's Refunder: all
// three admin routes -- List, Get, Refund -- land on one AdminHandler now,
// so they share one port named for the capability set rather than for any
// one verb.
type PaymentManager interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	ListAdmin(ctx context.Context, params payment.AdminListParams) ([]domain.Payment, int, error)
	Refund(ctx context.Context, paymentID uuid.UUID) error
}

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

type AdminHandler struct {
	service PaymentManager
}

func NewAdminHandler(service PaymentManager) *AdminHandler {
	return &AdminHandler{service: service}
}

func (h *AdminHandler) List(w http.ResponseWriter, r *http.Request) {
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

func (h *AdminHandler) Get(w http.ResponseWriter, r *http.Request) {
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

func (h *AdminHandler) Refund(w http.ResponseWriter, r *http.Request) {
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
