package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/inventory"
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

	Availability inventory.Availability
}

type Image struct {
	ID        uuid.UUID
	ProductID uuid.UUID
	URL       string
	AltText   *string
	SortOrder int
	CreatedAt time.Time
}
