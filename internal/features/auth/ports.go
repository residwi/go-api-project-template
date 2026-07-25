package auth

import (
	"context"

	"github.com/google/uuid"
)

type UserCredentials struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
	Role         string
	Active       bool
	TokenVersion int
}

type UserResult struct {
	ID           uuid.UUID
	Email        string
	FirstName    string
	LastName     string
	Role         string
	Active       bool
	TokenVersion int
}

type CreateUserParams struct {
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
}

type UserProvider interface {
	GetByEmail(ctx context.Context, email string) (UserCredentials, error)
	Create(ctx context.Context, params CreateUserParams) (UserResult, error)
	GetByID(ctx context.Context, id uuid.UUID) (UserResult, error)
}
