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
	ID             uuid.UUID  `json:"id"`
	CategoryID     *uuid.UUID `json:"category_id,omitempty"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	Description    *string    `json:"description,omitempty"`
	Price          int64      `json:"price"`
	CompareAtPrice *int64     `json:"compare_at_price,omitempty"`
	Currency       string     `json:"currency"`
	SKU            *string    `json:"sku,omitempty"`
	Status         string     `json:"status"`
	Images         []Image    `json:"images,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	DeletedAt      *time.Time `json:"-"`

	// Availability is filled by Service from the InventoryReader port, not by
	// the repository: inventory owns these numbers and products has no such
	// columns.
	Availability Availability `json:"-"`
}

type Image struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	URL       string    `json:"url"`
	AltText   *string   `json:"alt_text,omitempty"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}
