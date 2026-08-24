package auth

import "github.com/google/uuid"

type ClaimsView struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string
	TokenVersion int
}
