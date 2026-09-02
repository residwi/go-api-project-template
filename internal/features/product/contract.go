package product

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/money"
)

type Info struct {
	ID        uuid.UUID
	Name      string
	Price     money.Money
	Status    string
	Available int
}
