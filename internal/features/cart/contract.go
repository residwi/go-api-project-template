package cart

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

type Snapshot struct {
	ID    uuid.UUID
	Items []Item
}

type Item struct {
	ProductID uuid.UUID
	Quantity  int
	Name      string
	Price     money.Money
	Status    string
}
