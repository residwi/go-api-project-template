package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type PaymentManager interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	ListAdmin(ctx context.Context, params payment.AdminListParams) ([]domain.Payment, int, error)
	Refund(ctx context.Context, paymentID uuid.UUID) error
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
