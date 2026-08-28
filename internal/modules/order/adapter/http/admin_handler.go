package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type OrderManager interface {
	ListAdmin(ctx context.Context, params order.AdminListParams) ([]domain.Order, int, error)
	Get(ctx context.Context, orderID uuid.UUID) (*domain.Order, error)
	ChangeStatus(ctx context.Context, orderID uuid.UUID, toStatus domain.Status) error
}

type AdminHandler struct {
	service   OrderManager
	validator *validator.Validator
}

func NewAdminHandler(service OrderManager, v *validator.Validator) *AdminHandler {
	return &AdminHandler{service: service, validator: v}
}

func (h *AdminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := order.AdminListParams{
		OffsetPage: page,
		Status:     r.URL.Query().Get("status"),
	}

	orders, total, err := h.service.ListAdmin(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]orderResponse, len(orders))
	for i, o := range orders {
		out[i] = toOrderResponse(&o)
	}

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
}

func (h *AdminHandler) Get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	o, err := h.service.Get(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toOrderResponse(o))
}

func (h *AdminHandler) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateStatusRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.ChangeStatus(r.Context(), id, domain.Status(req.Status)); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
