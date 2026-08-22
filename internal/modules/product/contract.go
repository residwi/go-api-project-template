package product

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// ProductInfo is the published lookup shape cart's ProductLookup port reads
// a product through. Named Info, not Product, because domain.Product
// already names the richer type this package builds it from.
//
//nolint:revive // stutter is deliberate: bare Product would collide in meaning with domain.Product
type ProductInfo struct {
	ID        uuid.UUID
	Name      string
	Price     money.Money
	Status    string
	Available int
}
