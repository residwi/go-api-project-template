package domain

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/user"
)

type Claims struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string
	TokenVersion int
}

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	User         user.Profile
}
