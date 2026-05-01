package product

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

type Product struct {
	ID               uuid.UUID  `json:"id"`
	CategoryID       *uuid.UUID `json:"category_id,omitempty"`
	Name             string     `json:"name"`
	Slug             string     `json:"slug"`
	Description      *string    `json:"description,omitempty"`
	Price            int64      `json:"price"`
	CompareAtPrice   *int64     `json:"compare_at_price,omitempty"`
	Currency         string     `json:"currency"`
	SKU              *string    `json:"sku,omitempty"`
	StockQuantity    int        `json:"stock_quantity"`
	ReservedQuantity int        `json:"reserved_quantity"`
	Status           string     `json:"status"`
	Images           []Image    `json:"images,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
	DeletedAt        *time.Time `json:"-"`
}

type Image struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	URL       string    `json:"url"`
	AltText   *string   `json:"alt_text,omitempty"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}
