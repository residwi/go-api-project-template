package auth

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/user"
)

type UserDirectory interface {
	GetByEmail(ctx context.Context, email string) (user.Credentials, error)
	Create(ctx context.Context, p user.NewUser) (user.Profile, error)
	GetByID(ctx context.Context, id uuid.UUID) (user.Profile, error)
}
