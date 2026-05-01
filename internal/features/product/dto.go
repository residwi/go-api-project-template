package product

import "github.com/google/uuid"

type CreateProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id" validate:"omitempty"`
	Name           string     `json:"name" validate:"required,min=1,max=255"`
	Description    *string    `json:"description" validate:"omitempty"`
	Price          int64      `json:"price" validate:"required,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       string     `json:"currency" validate:"omitempty,len=3"`
	SKU            *string    `json:"sku" validate:"omitempty,max=100"`
	StockQuantity  *int       `json:"stock_quantity" validate:"omitempty,min=0"`
	Status         string     `json:"status" validate:"omitempty,oneof=draft published archived"`
}

type UpdateProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id" validate:"omitempty"`
	Name           *string    `json:"name" validate:"omitempty,min=1,max=255"`
	Description    *string    `json:"description" validate:"omitempty"`
	Price          *int64     `json:"price" validate:"omitempty,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       *string    `json:"currency" validate:"omitempty,len=3"`
	SKU            *string    `json:"sku" validate:"omitempty,max=100"`
	StockQuantity  *int       `json:"stock_quantity" validate:"omitempty,min=0"`
	Status         *string    `json:"status" validate:"omitempty,oneof=draft published archived"`
}

type AddImageRequest struct {
	URL       string  `json:"url" validate:"required,url"`
	AltText   *string `json:"alt_text" validate:"omitempty,max=255"`
	SortOrder *int    `json:"sort_order" validate:"omitempty,min=0"`
}

type Response struct {
	Product *Product `json:"product"`
}

type ListResponse struct {
	Products []Product `json:"products"`
}
