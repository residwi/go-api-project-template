package domain

import (
	"github.com/google/uuid"

	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
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
	User         usercontract.User
}
