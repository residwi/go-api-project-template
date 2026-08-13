package register

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

type UserCreator interface {
	Create(ctx context.Context, p usercontract.NewUser) (usercontract.User, error)
}

type TokenIssuer interface {
	BuildTokenPair(user usercontract.User) (*domain.TokenPair, error)
}
