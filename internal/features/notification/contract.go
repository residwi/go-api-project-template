package notification

import "github.com/google/uuid"

type NewNotification struct {
	UserID uuid.UUID
	Title  string
	Body   string
}
