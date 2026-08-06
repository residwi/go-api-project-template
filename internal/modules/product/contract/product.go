// Package contract is product's published surface. It imports no module and no
// platform package.
package contract

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// Product is what other modules need to price and validate a product line.
// Status already accounts for withdrawal: see product.Service.GetInfoByIDs.
type Product struct {
	ID        uuid.UUID
	Name      string
	Price     money.Money
	Status    string
	Available int
}
