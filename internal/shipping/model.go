package shipping

import (
	"time"

	"github.com/google/uuid"
)

type ShipmentStatus string

const (
	StatusPending   ShipmentStatus = "pending"
	StatusShipped   ShipmentStatus = "shipped"
	StatusInTransit ShipmentStatus = "in_transit"
	StatusDelivered ShipmentStatus = "delivered"
	StatusReturned  ShipmentStatus = "returned"
)

type Shipment struct {
	ID             uuid.UUID      `json:"id"`
	OrderID        uuid.UUID      `json:"order_id"`
	Carrier        string         `json:"carrier,omitempty"`
	TrackingNumber string         `json:"tracking_number,omitempty"`
	Status         ShipmentStatus `json:"status"`
	ShippedAt      *time.Time     `json:"shipped_at,omitempty"`
	DeliveredAt    *time.Time     `json:"delivered_at,omitempty"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}
