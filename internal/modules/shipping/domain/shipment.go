package domain

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
	ID             uuid.UUID
	OrderID        uuid.UUID
	Carrier        string
	TrackingNumber string
	Status         ShipmentStatus
	ShippedAt      *time.Time
	DeliveredAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

func CanShipOrder(orderStatus string) bool {
	return orderStatus == "paid" || orderStatus == "processing"
}
