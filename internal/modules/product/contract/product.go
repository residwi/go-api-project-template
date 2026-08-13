package contract

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

type Product struct {
	ID        uuid.UUID
	Name      string
	Price     money.Money
	Status    string
	Available int
}
