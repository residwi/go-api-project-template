package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/create"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// ShipmentCreator is what Handler needs from create.Command: create.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in handler_test.go.
type ShipmentCreator interface {
	Execute(ctx context.Context, orderID uuid.UUID, p create.Params) (*domain.Shipment, error)
}

type Handler struct {
	cmd       ShipmentCreator
	validator *validator.Validator
}

func New(cmd ShipmentCreator, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("POST /orders/{id}/ship", h.create)
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

// Declared here, not shared with shipping's other slices. Each endpoint holds
// its own copy so one endpoint's new field cannot appear in another's response.
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

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	orderID, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[createShipmentRequest](w, r, h.validator)
	if !ok {
		return
	}

	shipment, err := h.cmd.Execute(r.Context(), orderID, req.toParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toShipmentResponse(shipment))
}
