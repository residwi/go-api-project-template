package shipping

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/middleware"
)

type publicHandler struct {
	service *Service
	orders  OrderProvider
}

func (h *publicHandler) GetShipping(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.GetUserContext(r.Context())
	if !ok {
		response.Unauthorized(w, "authentication required")
		return
	}

	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	order, err := h.orders.GetByID(r.Context(), orderID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	if order.UserID != uc.UserID {
		response.HandleErr(w, core.ErrNotFound)
		return
	}

	shipment, err := h.service.GetByOrderID(r.Context(), orderID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, shipment)
}
