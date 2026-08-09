package register

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

// UserCreator is satisfied directly by credentials.Store.Create -- no adapter.
type UserCreator interface {
	Create(ctx context.Context, p usercontract.NewUser) (usercontract.User, error)
}

// TokenIssuer is satisfied directly by token.Service.BuildTokenPair -- no
// adapter.
type TokenIssuer interface {
	BuildTokenPair(user usercontract.User) (*domain.TokenPair, error)
}
