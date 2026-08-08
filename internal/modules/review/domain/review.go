// Package domain holds review's aggregate. It is module-private: what leaves
// review leaves through a slice's return type.
package domain

import (
	"time"

	"github.com/google/uuid"
)

const (
	StatusPending   = "pending"
	StatusPublished = "published"
	StatusRejected  = "rejected"
)

type Review struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	ProductID uuid.UUID
	OrderID   uuid.UUID
	Rating    int
	Title     string
	Body      string
	Status    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Stats struct {
	AverageRating float64
	TotalReviews  int
}
