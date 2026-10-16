package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type ShipmentManager interface {
	Create(ctx context.Context, orderID uuid.UUID, carrier, trackingNumber string) (*domain.Shipment, error)
	Deliver(ctx context.Context, shipmentID uuid.UUID) (*domain.Shipment, error)
	UpdateTracking(ctx context.Context, shipmentID uuid.UUID, carrier, trackingNumber string) (*domain.Shipment, error)
}

type AdminHandler struct {
	service ShipmentManager
}

func NewAdminHandler(service ShipmentManager) *AdminHandler {
	return &AdminHandler{service: service}
}

func (h *AdminHandler) Create(w http.ResponseWriter, r *http.Request) error {
	orderID, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	req, err := request.Bind[createShipmentRequest](w, r)
	if err != nil {
		return err
	}

	shipment, err := h.service.Create(r.Context(), orderID, req.Carrier, req.TrackingNumber)
	if err != nil {
		return err
	}

	response.Created(w, toShipmentResponse(shipment))
	return nil
}

func (h *AdminHandler) Deliver(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	shipment, err := h.service.Deliver(r.Context(), id)
	if err != nil {
		return err
	}

	response.OK(w, toShipmentResponse(shipment))
	return nil
}

func (h *AdminHandler) UpdateTracking(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	req, err := request.Bind[updateTrackingRequest](w, r)
	if err != nil {
		return err
	}

	shipment, err := h.service.UpdateTracking(r.Context(), id, req.Carrier, req.TrackingNumber)
	if err != nil {
		return err
	}

	response.OK(w, toShipmentResponse(shipment))
	return nil
}
