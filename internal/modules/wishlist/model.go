package wishlist

import (
	"time"

	"github.com/google/uuid"
)

type Wishlist struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Items     []Item
	CreatedAt time.Time
}

type Item struct {
	ID         uuid.UUID
	WishlistID uuid.UUID
	ProductID  uuid.UUID
	CreatedAt  time.Time
}
