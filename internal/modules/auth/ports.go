package auth

import (
	"context"

	"github.com/google/uuid"

	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

// UserProvider is satisfied directly by user.Service -- same method names,
// contract types.
type UserProvider interface {
	GetByEmail(ctx context.Context, email string) (usercontract.Credentials, error)
	Create(ctx context.Context, p usercontract.NewUser) (usercontract.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (usercontract.User, error)
}
