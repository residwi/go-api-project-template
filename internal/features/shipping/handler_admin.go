package shipping

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
)

type adminHandler struct {
	service   *Service
	validator *validator.Validator
}

func (h *adminHandler) CreateShipment(w http.ResponseWriter, r *http.Request) {
	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var req CreateShipmentRequest
	if decodeErr := response.DecodeJSON(w, r, &req); decodeErr != nil {
		response.HandleErr(w, decodeErr)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
		return
	}

	shipment, err := h.service.CreateShipment(r.Context(), orderID, req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, shipment)
}

func (h *adminHandler) UpdateTracking(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	var req UpdateTrackingRequest
	if decodeErr := response.DecodeJSON(w, r, &req); decodeErr != nil {
		response.HandleErr(w, decodeErr)
		return
	}

	if errors := h.validator.Validate(req); errors != nil {
		response.ValidationErr(w, errors)
		return
	}

	shipment, err := h.service.UpdateTracking(r.Context(), id, req)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, shipment)
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

	response.OK(w, shipment)
}
