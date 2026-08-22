package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type ShipmentManager interface {
	Create(ctx context.Context, orderID uuid.UUID, carrier, trackingNumber string) (*domain.Shipment, error)
	Deliver(ctx context.Context, shipmentID uuid.UUID) (*domain.Shipment, error)
	UpdateTracking(ctx context.Context, shipmentID uuid.UUID, carrier, trackingNumber string) (*domain.Shipment, error)
}

type AdminHandler struct {
	service   ShipmentManager
	validator *validator.Validator
}

func NewAdminHandler(service ShipmentManager, v *validator.Validator) *AdminHandler {
	return &AdminHandler{service: service, validator: v}
}

type createShipmentRequest struct {
	Carrier        string `json:"carrier"         validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}

func (h *AdminHandler) Create(w http.ResponseWriter, r *http.Request) {
	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[createShipmentRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.service.Create(r.Context(), orderID, req.Carrier, req.TrackingNumber)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toShipmentResponse(shipment))
}

func (h *AdminHandler) Deliver(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	shipment, err := h.service.Deliver(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}

type updateTrackingRequest struct {
	Carrier        string `json:"carrier"         validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}

func (h *AdminHandler) UpdateTracking(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateTrackingRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.service.UpdateTracking(r.Context(), id, req.Carrier, req.TrackingNumber)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}
