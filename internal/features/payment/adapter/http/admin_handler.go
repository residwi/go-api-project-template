package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/features/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
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

func (h *AdminHandler) List(w http.ResponseWriter, r *http.Request) error {
	page := paging.ParseOffsetPage(r)
	params := payment.AdminListParams{
		OffsetPage: page,
		Status:     r.URL.Query().Get("status"),
		OrderID:    r.URL.Query().Get("order_id"),
	}

	payments, total, err := h.service.ListAdmin(r.Context(), params)
	if err != nil {
		return err
	}

	out := make([]adminPaymentResponse, len(payments))
	for i, p := range payments {
		out[i] = toAdminPaymentResponse(&p)
	}

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
	return nil
}

func (h *AdminHandler) GetByID(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	p, err := h.service.GetByID(r.Context(), id)
	if err != nil {
		return err
	}

	response.OK(w, toAdminPaymentResponse(p))
	return nil
}

func (h *AdminHandler) Refund(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	if err := h.service.Refund(r.Context(), id); err != nil {
		return err
	}

	response.OK(w, refundResponse{Status: "refund_enqueued"})
	return nil
}
