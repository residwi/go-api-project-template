package auth

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/auth/usecase/login"
	"github.com/residwi/go-api-project-template/internal/modules/auth/usecase/refresh"
	"github.com/residwi/go-api-project-template/internal/modules/auth/usecase/register"
	"github.com/residwi/go-api-project-template/internal/modules/auth/usecase/token"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

type Deps struct {
	Config Config
	Users  UserPorts
}

type UserPorts interface {
	GetByEmail(ctx context.Context, email string) (usercontract.Credentials, error)
	Create(ctx context.Context, p usercontract.NewUser) (usercontract.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (usercontract.User, error)
}

type Module struct {
	Register *register.Command
	Login    *login.Command
	Refresh  *refresh.Command
	Token    *token.Service
}

func New(d Deps) *Module {
	tok := token.New(d.Config.Secret, d.Config.Issuer, d.Config.AccessTokenTTL, d.Config.RefreshTokenTTL)

	return &Module{
		Register: register.New(d.Users, tok, d.Config.BcryptCost),
		Login:    login.New(d.Users, tok, d.Config.BcryptCost),
		Refresh:  refresh.New(d.Users, tok, tok),
		Token:    tok,
	}
}
