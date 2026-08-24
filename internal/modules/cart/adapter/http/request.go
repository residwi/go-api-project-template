package http

import "github.com/google/uuid"

type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Quantity  int       `json:"quantity"   validate:"required,min=1"`
}

type updateQuantityRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}
