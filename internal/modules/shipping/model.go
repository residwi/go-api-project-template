package shipping

import "github.com/residwi/go-api-project-template/internal/modules/shipping/domain"

// Aliases keep service.go, postgres/ and http/ compiling while the slices
// are extracted one at a time. Task 8 deletes this file along with the husk.
type (
	// Shipment aliases domain.Shipment.
	Shipment = domain.Shipment
	// ShipmentStatus aliases domain.ShipmentStatus.
	ShipmentStatus = domain.ShipmentStatus
)

const (
	StatusPending   = domain.StatusPending
	StatusShipped   = domain.StatusShipped
	StatusInTransit = domain.StatusInTransit
	StatusDelivered = domain.StatusDelivered
	StatusReturned  = domain.StatusReturned
)
