package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/order/query"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// AdminReader is what AdminHandler needs from query.Reader: query.Reader
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in
// admin_handler_test.go.
type AdminReader interface {
	ListAdmin(ctx context.Context, params query.AdminListParams) ([]domain.Order, int, error)
	GetByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, error)
}

type AdminHandler struct {
	reader AdminReader
}

func NewAdmin(reader AdminReader) *AdminHandler {
	return &AdminHandler{reader: reader}
}

func (h *AdminHandler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("GET /orders", h.list)
	admin.HandleFunc("GET /orders/{id}", h.get)
}

func (h *AdminHandler) list(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := query.AdminListParams{
		OffsetPage: page,
		Status:     r.URL.Query().Get("status"),
	}

	orders, total, err := h.reader.ListAdmin(r.Context(), params)
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

func (h *AdminHandler) get(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	o, err := h.reader.GetByID(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toOrderResponse(o))
}
