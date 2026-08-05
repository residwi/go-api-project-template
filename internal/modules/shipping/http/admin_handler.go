package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service   *shipping.Service
	validator *validator.Validator
}

type createShipmentRequest struct {
	Carrier        string `json:"carrier"         validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}

func (r createShipmentRequest) toCreateParams() shipping.CreateParams {
	return shipping.CreateParams{
		Carrier:        r.Carrier,
		TrackingNumber: r.TrackingNumber,
	}
}

func (h *adminHandler) CreateShipment(w http.ResponseWriter, r *http.Request) {
	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[createShipmentRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.service.CreateShipment(r.Context(), orderID, req.toCreateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toShipmentResponse(shipment))
}

type updateTrackingRequest struct {
	Carrier        string `json:"carrier"         validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}

func (r updateTrackingRequest) toUpdateTrackingParams() shipping.UpdateTrackingParams {
	return shipping.UpdateTrackingParams{
		Carrier:        r.Carrier,
		TrackingNumber: r.TrackingNumber,
	}
}

func (h *adminHandler) UpdateTracking(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateTrackingRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.service.UpdateTracking(r.Context(), id, req.toUpdateTrackingParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}

func (h *adminHandler) MarkDelivered(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	shipment, err := h.service.MarkDelivered(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}
