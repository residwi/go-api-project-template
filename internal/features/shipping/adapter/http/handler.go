package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/shipping/domain"
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

func (h *Handler) GetForUser(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	orderID, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	shipment, err := h.service.GetForUser(r.Context(), uc.UserID, orderID)
	if err != nil {
		return err
	}

	response.OK(w, toShipmentResponse(shipment))
	return nil
}
