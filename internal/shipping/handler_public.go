package shipping

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type publicHandler struct {
	service *Service
	orders  OrderProvider
}

func (h *publicHandler) GetShipping(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
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
		response.HandleErr(w, apperror.ErrNotFound)
		return
	}

	shipment, err := h.service.GetByOrderID(r.Context(), orderID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, shipment)
}
