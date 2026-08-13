package contract

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

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
