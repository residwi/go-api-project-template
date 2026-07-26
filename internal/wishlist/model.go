package wishlist

import (
	"time"

	"github.com/google/uuid"
)

type Wishlist struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Items     []Item    `json:"items,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

type Item struct {
	ID         uuid.UUID `json:"id"`
	WishlistID uuid.UUID `json:"-"`
	ProductID  uuid.UUID `json:"product_id"`
	CreatedAt  time.Time `json:"created_at"`
}
