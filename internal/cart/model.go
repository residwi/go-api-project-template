package cart

import (
	"time"

	"github.com/google/uuid"
)

type Cart struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Items     []Item    `json:"items"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Item struct {
	ID        uuid.UUID `json:"id"`
	CartID    uuid.UUID `json:"-"`
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int       `json:"quantity"`
	Product   *Product  `json:"product,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Product struct {
	Name     string `json:"name"`
	Price    int64  `json:"price"`
	Currency string `json:"currency"`
	Stock    int    `json:"available_stock"`
	Status   string `json:"status"`
}
