package cart

import "github.com/google/uuid"

type AddItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Quantity  int       `json:"quantity" validate:"required,min=1"`
}

type UpdateItemRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}
