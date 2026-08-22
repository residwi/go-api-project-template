package auth

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user"
)

// UserDirectory is everything auth asks user for: it owns no store of its
// own, so login, registration and refresh all read and write through this
// one port instead. Satisfied by user's Service by name-match.
type UserDirectory interface {
	GetByEmail(ctx context.Context, email string) (user.Credentials, error)
	Create(ctx context.Context, p user.NewUser) (user.Profile, error)
	GetByID(ctx context.Context, id uuid.UUID) (user.Profile, error)
}
