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
