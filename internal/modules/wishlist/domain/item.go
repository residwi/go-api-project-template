// Package domain holds wishlist's aggregate. It is module-private: what
// leaves wishlist leaves through a slice's return type.
package domain

import (
	"time"

	"github.com/google/uuid"
)

type Item struct {
	ID         uuid.UUID
	WishlistID uuid.UUID
	ProductID  uuid.UUID
	CreatedAt  time.Time
}
