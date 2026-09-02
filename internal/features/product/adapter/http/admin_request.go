package http

import (
	"github.com/google/uuid"
)

type createProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id"      validate:"omitempty"`
	Name           string     `json:"name"             validate:"required,min=1,max=255"`
	Description    *string    `json:"description"      validate:"omitempty"`
	Price          int64      `json:"price"            validate:"required,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       string     `json:"currency"         validate:"omitempty,len=3"`
	SKU            *string    `json:"sku"              validate:"omitempty,max=100"`
	Status         string     `json:"status"           validate:"omitempty,oneof=draft published archived"`
}

type updateProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id"      validate:"omitempty"`
	Name           *string    `json:"name"             validate:"omitempty,min=1,max=255"`
	Description    *string    `json:"description"      validate:"omitempty"`
	Price          *int64     `json:"price"            validate:"omitempty,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       *string    `json:"currency"         validate:"omitempty,len=3"`
	SKU            *string    `json:"sku"              validate:"omitempty,max=100"`
	Status         *string    `json:"status"           validate:"omitempty,oneof=draft published archived"`
}
