package product

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

type Product struct {
	ID          uuid.UUID
	CategoryID  *uuid.UUID
	Name        string
	Slug        string
	Description *string
	// Price and CompareAtPrice are denominated in the same currency: the products
	// table stores one currency column for both, so they cannot differ on a row
	// and nothing may set them independently. Pairing each amount with that
	// currency means a comparison between them -- or against a cart line's price
	// -- cannot silently mix denominations, and the http adapter reads the
	// product's currency off Price.
	Price          money.Money
	CompareAtPrice *money.Money
	SKU            *string
	Status         string
	Images         []Image
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time

	// Availability is filled by Service from the InventoryReader port, not by
	// the repository: inventory owns these numbers and products has no such
	// columns.
	Availability Availability
}

type Image struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	URL       string
	AltText   *string
	SortOrder int
	CreatedAt time.Time
}
