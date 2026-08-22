package auth

import (
	"context"

	"github.com/google/uuid"

	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

// UserDirectory is everything auth asks user for: it owns no store of its
// own, so login, registration and refresh all read and write through this
// one port instead. Satisfied by user's credentials use case by name-match.
type UserDirectory interface {
	GetByEmail(ctx context.Context, email string) (usercontract.Credentials, error)
	Create(ctx context.Context, p usercontract.NewUser) (usercontract.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (usercontract.User, error)
}
