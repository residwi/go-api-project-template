// Package domain holds product's aggregate and its rules. It is
// module-private: what leaves product leaves through a slice's return type or
// contract/.
package domain

import (
	"time"

	"github.com/google/uuid"

	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/money"
)

const (
	StatusDraft     = "draft"
	StatusPublished = "published"
	StatusArchived  = "archived"
)

type Product struct {
	ID             uuid.UUID
	CategoryID     *uuid.UUID
	Name           string
	Slug           string
	Description    *string
	Price          money.Money
	CompareAtPrice *money.Money
	SKU            *string
	Status         string
	Images         []Image
	CreatedAt      time.Time
	UpdatedAt      time.Time
	DeletedAt      *time.Time

	// Filled by query's Reader from the InventoryReader port: inventory owns
	// these numbers and product has no such columns.
	Availability inventorycontract.Availability
}

type Image struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	URL       string
	AltText   *string
	SortOrder int
	CreatedAt time.Time
}
