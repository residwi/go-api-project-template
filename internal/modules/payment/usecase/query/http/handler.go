package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/usecase/query"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type PaymentReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	ListAdmin(ctx context.Context, params query.AdminListParams) ([]domain.Payment, int, error)
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

type Handler struct {
	reader PaymentReader
}

func New(reader PaymentReader) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := query.AdminListParams{
		OffsetPage: page,
		Status:     r.URL.Query().Get("status"),
		OrderID:    r.URL.Query().Get("order_id"),
	}

	payments, total, err := h.reader.ListAdmin(r.Context(), params)
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

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	p, err := h.reader.GetByID(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminPaymentResponse(p))
}
