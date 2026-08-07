// Package domain holds shipping's aggregate and its rules. It is module-private:
// no other module may import it, because a field added here would otherwise
// become another module's problem. What leaves shipping leaves through a slice's
// return type.
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

// CanShipOrder reports whether an order in this status may have a shipment
// created against it. The strings are order's status values, read through
// ordercontract.Order -- shipping compares them but does not own them.
func CanShipOrder(orderStatus string) bool {
	return orderStatus == "paid" || orderStatus == "processing"
}
