package product

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// Slug is not a param: it is always derived server-side from Name via
// slug.MakeOrFallback, both on create and on a name-changing update.

// CreateParams may leave Price's currency empty: Service.Create denominates it.
// CompareAtPrice is denominated from Price, never independently.
type CreateParams struct {
	CategoryID     *uuid.UUID
	Name           string
	Description    *string
	Price          money.Money
	CompareAtPrice *money.Money
	SKU            *string
	Status         string
}

// UpdateParams reads nil as "leave it alone". Re-pricing requires a currency
// too; admin_handler.go rejects one without the other.
type UpdateParams struct {
	CategoryID     *uuid.UUID
	Name           *string
	Description    *string
	Price          *money.Money
	CompareAtPrice *money.Money
	SKU            *string
	Status         *string
}

type AddImageParams struct {
	URL       string
	AltText   *string
	SortOrder *int
}
