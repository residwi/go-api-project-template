package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type OrderReader interface {
	ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Order, error)
	GetForUser(ctx context.Context, userID, orderID uuid.UUID) (*domain.Order, error)
}

type Handler struct {
	service OrderReader
}

func NewHandler(service OrderReader) *Handler {
	return &Handler{service: service}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	uc, ok := request.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	orders, err := h.service.ListByUser(r.Context(), uc.UserID, cursor)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]orderResponse, len(orders))
	for i, o := range orders {
		out[i] = toOrderResponse(&o)
	}

	response.CursorPage(w, out, cursor.Limit, func(o orderResponse) (time.Time, uuid.UUID) {
		return o.CreatedAt, o.ID
	})
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uc, ok := request.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	o, err := h.service.GetForUser(r.Context(), uc.UserID, id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toOrderResponse(o))
}
