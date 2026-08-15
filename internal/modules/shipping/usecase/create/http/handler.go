package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/create"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type ShipmentCreator interface {
	Execute(ctx context.Context, orderID uuid.UUID, p create.Params) (*domain.Shipment, error)
}

type Handler struct {
	usecase   ShipmentCreator
	validator *validator.Validator
}

func New(usecase ShipmentCreator, v *validator.Validator) *Handler {
	return &Handler{usecase: usecase, validator: v}
}

type createShipmentRequest struct {
	Carrier        string `json:"carrier"         validate:"required"`
	TrackingNumber string `json:"tracking_number" validate:"required"`
}

func (r createShipmentRequest) toParams() create.Params {
	return create.Params{
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

func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[createShipmentRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.usecase.Execute(r.Context(), orderID, req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toShipmentResponse(shipment))
}
