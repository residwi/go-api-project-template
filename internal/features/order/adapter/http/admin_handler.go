package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type OrderManager interface {
	ListAdmin(ctx context.Context, params order.AdminListParams) ([]domain.Order, int, error)
	Get(ctx context.Context, orderID uuid.UUID) (*domain.Order, error)
	ChangeStatus(ctx context.Context, orderID uuid.UUID, toStatus domain.Status) error
}

type AdminHandler struct {
	service OrderManager
}

func NewAdminHandler(service OrderManager) *AdminHandler {
	return &AdminHandler{service: service}
}

func (h *AdminHandler) List(w http.ResponseWriter, r *http.Request) error {
	page := paging.ParseOffsetPage(r)
	params := order.AdminListParams{
		OffsetPage: page,
		Status:     r.URL.Query().Get("status"),
	}

	orders, total, err := h.service.ListAdmin(r.Context(), params)
	if err != nil {
		return err
	}

	out := make([]orderResponse, len(orders))
	for i, o := range orders {
		out[i] = toOrderResponse(&o)
	}

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
	return nil
}

func (h *AdminHandler) Get(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	o, err := h.service.Get(r.Context(), id)
	if err != nil {
		return err
	}

	response.OK(w, toOrderResponse(o))
	return nil
}

func (h *AdminHandler) ChangeStatus(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	req, err := request.Bind[updateStatusRequest](w, r)
	if err != nil {
		return err
	}

	if err := h.service.ChangeStatus(r.Context(), id, domain.Status(req.Status)); err != nil {
		return err
	}

	response.NoContent(w)
	return nil
}
