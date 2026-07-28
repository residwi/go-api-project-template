package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/shipping"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// updateTrackingRequest carries both fields verbatim from the deleted
// dto.go's UpdateTrackingRequest -- the service updates either
// independently when supplied non-empty, not just TrackingNumber.
type updateTrackingRequest struct {
	Carrier        string `json:"carrier" validate:"required"`
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
