package product

import "github.com/google/uuid"

// Params types are the service's input contract. They carry no json or
// validate tags: those belong to a transport, and this service is also
// reachable from places that have no HTTP request to validate. Each
// transport maps its own wire type onto these.
//
// Slug is not a param: it is always derived server-side from Name via
// slug.MakeOrFallback, both on create and on a name-changing update. A
// client-supplied slug was never accepted by the pre-refactor
// CreateProductRequest/UpdateProductRequest either.

type CreateParams struct {
	CategoryID     *uuid.UUID
	Name           string
	Description    *string
	Price          int64
	CompareAtPrice *int64
	Currency       string
	SKU            *string
	Status         string
}

type UpdateParams struct {
	CategoryID     *uuid.UUID
	Name           *string
	Description    *string
	Price          *int64
	CompareAtPrice *int64
	Currency       *string
	SKU            *string
	Status         *string
}

type AddImageParams struct {
	URL       string
	AltText   *string
	SortOrder *int
}
