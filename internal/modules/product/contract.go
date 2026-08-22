package product

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

// Info is the published lookup shape cart's ProductLookup port reads a
// product through.
type Info struct {
	ID        uuid.UUID
	Name      string
	Price     money.Money
	Status    string
	Available int
}
