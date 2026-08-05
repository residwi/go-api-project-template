package cart

import "github.com/google/uuid"

type AddItemParams struct {
	ProductID uuid.UUID
	Quantity  int
}

type UpdateQuantityParams struct {
	Quantity int
}
