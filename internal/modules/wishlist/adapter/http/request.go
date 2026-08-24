package http

import "github.com/google/uuid"

type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
}
