package product

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// Slug is not a param: it is always derived server-side from Name via
// slug.MakeOrFallback, both on create and on a name-changing update. A
// client-supplied slug was never accepted by the pre-refactor
// CreateProductRequest/UpdateProductRequest either.

// CreateParams may leave Price's currency empty: Service.Create denominates it
// in defaultCurrency, which is where that default has always lived.
// CompareAtPrice is denominated from Price, never independently -- see
// Product.Price.
type CreateParams struct {
	CategoryID     *uuid.UUID
	Name           string
	Description    *string
	Price          money.Money
	CompareAtPrice *money.Money
	SKU            *string
	Status         string
}

// UpdateParams treats Price and CompareAtPrice as optional as a whole -- nil
// means "leave it alone" -- but an amount is no longer separable from its
// currency, so there is no way to re-price a product without saying what the new
// price is denominated in. That is deliberate;
// internal/modules/product/http/admin_handler.go rejects the requests it makes
// unrepresentable rather than guessing a currency.
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
