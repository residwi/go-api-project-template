package domain

import "github.com/google/uuid"

type Stock struct {
	ProductID uuid.UUID
	Quantity  int
	Reserved  int
	Available int
}
