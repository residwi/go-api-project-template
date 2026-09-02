package domain

import (
	"time"

	"github.com/google/uuid"
)

type Notification struct {
	ID        uuid.UUID
	UserID    uuid.UUID
	Title     string
	Body      string
	IsRead    bool
	CreatedAt time.Time
}
