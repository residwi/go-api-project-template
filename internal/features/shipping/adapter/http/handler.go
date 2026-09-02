package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
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

	orderID, ok := request.ParseUUIDParam(w, r, "id")
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
