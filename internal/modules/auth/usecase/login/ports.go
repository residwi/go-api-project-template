package login

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

type UserProvider interface {
	GetByEmail(ctx context.Context, email string) (usercontract.Credentials, error)
}

type TokenIssuer interface {
	BuildTokenPair(user usercontract.User) (*domain.TokenPair, error)
}
