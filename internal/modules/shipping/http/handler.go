package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping"
)

// Mirrors shipping.Shipment 1:1: nothing on it is internal or sensitive. Kept
// here for the admin_handler.go methods; query/http declares its own copy for
// the authed route rather than sharing this one.
type shipmentResponse struct {
	ID             uuid.UUID               `json:"id"`
	OrderID        uuid.UUID               `json:"order_id"`
	Carrier        string                  `json:"carrier,omitempty"`
	TrackingNumber string                  `json:"tracking_number,omitempty"`
	Status         shipping.ShipmentStatus `json:"status"`
	ShippedAt      *time.Time              `json:"shipped_at,omitempty"`
	DeliveredAt    *time.Time              `json:"delivered_at,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
	UpdatedAt      time.Time               `json:"updated_at"`
}

func toShipmentResponse(s *shipping.Shipment) shipmentResponse {
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
