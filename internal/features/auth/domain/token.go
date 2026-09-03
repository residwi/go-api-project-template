package domain

import "github.com/google/uuid"

type Claims struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string
	TokenVersion int
}
