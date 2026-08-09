package login

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

// UserProvider is satisfied directly by credentials.Store.GetByEmail -- no adapter.
type UserProvider interface {
	GetByEmail(ctx context.Context, email string) (usercontract.Credentials, error)
}

// TokenIssuer is satisfied directly by token.Service.BuildTokenPair -- no
// adapter.
type TokenIssuer interface {
	BuildTokenPair(user usercontract.User) (*domain.TokenPair, error)
}
