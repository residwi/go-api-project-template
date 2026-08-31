package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
)

type ShipmentReader interface {
	GetForUser(ctx context.Context, userID, orderID uuid.UUID) (*domain.Shipment, error)
}

type Handler struct {
	service ShipmentReader
}

func NewHandler(service ShipmentReader) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	shipment, err := h.service.GetForUser(r.Context(), uc.UserID, orderID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}
