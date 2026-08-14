package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/updatetracking"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type TrackingUpdater interface {
	Execute(ctx context.Context, shipmentID uuid.UUID, p updatetracking.Params) (*domain.Shipment, error)
}

type Handler struct {
	cmd       TrackingUpdater
	validator *validator.Validator
}

func New(cmd TrackingUpdater, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

type updateTrackingRequest struct {
	Carrier        string `json:"carrier"         validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}

func (r updateTrackingRequest) toParams() updatetracking.Params {
	return updatetracking.Params{
		Carrier:        r.Carrier,
		TrackingNumber: r.TrackingNumber,
	}
}

type shipmentResponse struct {
	ID             uuid.UUID             `json:"id"`
	OrderID        uuid.UUID             `json:"order_id"`
	Carrier        string                `json:"carrier,omitempty"`
	TrackingNumber string                `json:"tracking_number,omitempty"`
	Status         domain.ShipmentStatus `json:"status"`
	ShippedAt      *time.Time            `json:"shipped_at,omitempty"`
	DeliveredAt    *time.Time            `json:"delivered_at,omitempty"`
	CreatedAt      time.Time             `json:"created_at"`
	UpdatedAt      time.Time             `json:"updated_at"`
}

func toShipmentResponse(s *domain.Shipment) shipmentResponse {
	return shipmentResponse{
		ID:             s.ID,
		OrderID:        s.OrderID,
		Carrier:        s.Carrier,
		TrackingNumber: s.TrackingNumber,
		Status:         s.Status,
		ShippedAt:      s.ShippedAt,
		DeliveredAt:    s.DeliveredAt,
		CreatedAt:      s.CreatedAt,
		UpdatedAt:      s.UpdatedAt,
	}
}

func (h *Handler) UpdateTracking(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateTrackingRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.cmd.Execute(r.Context(), id, req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}
