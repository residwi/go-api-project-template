package identity

import "github.com/google/uuid"

type Identity struct {
	UserID uuid.UUID
	Role   string
}
