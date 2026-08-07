package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// ShipmentDeliverer is what Handler needs from deliver.Command: deliver.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in handler_test.go.
type ShipmentDeliverer interface {
	Execute(ctx context.Context, shipmentID uuid.UUID) (*domain.Shipment, error)
}

// Handler holds no validator: the endpoint takes no body.
type Handler struct {
	cmd ShipmentDeliverer
}

func New(cmd ShipmentDeliverer) *Handler {
	return &Handler{cmd: cmd}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("POST /shipments/{id}/deliver", h.deliver)
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

func (h *Handler) deliver(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	shipment, err := h.cmd.Execute(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toShipmentResponse(shipment))
}
