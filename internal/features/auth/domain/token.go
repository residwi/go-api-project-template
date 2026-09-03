package domain

import "github.com/google/uuid"

type Kind string

const (
	AccessToken  Kind = "access"
	RefreshToken Kind = "refresh"
)

type Claims struct {
	UserID       uuid.UUID
	Role         string
	TokenVersion int
}
