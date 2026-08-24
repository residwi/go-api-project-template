package cart

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/money"
)

// Snapshot is cart's published shape for another module -- order's
// Cart port reads a user's cart through it. Named Snapshot, not Cart,
// because domain.Cart already names the richer type the cart module works
// with internally.
type Snapshot struct {
	ID    uuid.UUID
	Items []Item
}

// Item is one line inside a Snapshot.
type Item struct {
	ProductID uuid.UUID
	Quantity  int
	Name      string
	Price     money.Money
	Status    string
}
