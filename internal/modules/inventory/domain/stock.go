// Package domain holds inventory's aggregate. It is module-private: what
// leaves inventory leaves through a slice's return type or contract/.
package domain

import "github.com/google/uuid"

// Stock holds a product's inventory counts. Quantity is derived
// (Available + Reserved), not stored itself.
type Stock struct {
	ProductID uuid.UUID
	Quantity  int
	Reserved  int
	Available int
}
