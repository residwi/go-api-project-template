package order

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type publicHandler struct {
	service   *Service
	validator *validator.Validator
}

func (h *publicHandler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		response.BadRequest(w, "Idempotency-Key header is required")
		return
	}

	req, ok := response.Bind[PlaceOrderRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.PlaceOrder(r.Context(), uc.UserID, req, idempotencyKey)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, result)
}

func (h *publicHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := core.ParseCursorPage(r)

	orders, err := h.service.ListByUser(r.Context(), uc.UserID, cursor)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	hasMore := len(orders) > cursor.Limit
	if hasMore {
		orders = orders[:cursor.Limit]
	}

	var nextCursor string
	if hasMore && len(orders) > 0 {
		last := orders[len(orders)-1]
		nextCursor = core.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())
	}

	response.Paginated(w, core.NewCursorPageResult(orders, nextCursor, hasMore))
}

func (h *publicHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	order, err := h.service.GetByID(r.Context(), uc.UserID, id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, order)
}

func (h *publicHandler) RetryPayment(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[PayRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.service.RetryPayment(r.Context(), uc.UserID, id, req.PaymentMethodID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, result)
}

func (h *publicHandler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.CancelOrder(r.Context(), uc.UserID, id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
