// Package contract is cart's published surface. It imports no module and no
// platform package.
package contract

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// Cart is a cart frozen for checkout: enough to price and validate every line
// without a second call back into cart.
type Cart struct {
	ID    uuid.UUID
	Items []CartItem
}

type CartItem struct {
	ProductID uuid.UUID
	Quantity  int
	Name      string
	Price     money.Money
	Status    string
}
