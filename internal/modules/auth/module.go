// Package auth composes auth's slices. It imports no transport package, so a
// worker or a future grpc server can construct this module without linking
// HTTP. auth owns no table: none of its four slices has a repository.go or a
// postgres/ -- every one reaches user through a port instead.
package auth

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/auth/login"
	"github.com/residwi/go-api-project-template/internal/modules/auth/refresh"
	"github.com/residwi/go-api-project-template/internal/modules/auth/register"
	"github.com/residwi/go-api-project-template/internal/modules/auth/token"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

type Deps struct {
	Config Config

	// Users is user's service. It satisfies each slice's own port by
	// name-match, so no adapter stands between them.
	Users UserPorts
}

// UserPorts is the union of what auth's slices need from user. Each slice
// still declares its own narrow port; this exists so Deps has one field
// instead of one per slice.
type UserPorts interface {
	GetByEmail(ctx context.Context, email string) (usercontract.Credentials, error)
	Create(ctx context.Context, p usercontract.NewUser) (usercontract.User, error)
	GetByID(ctx context.Context, id uuid.UUID) (usercontract.User, error)
}

// Module is Token, Register, Login, Refresh. Token is exported because
// middleware.Auth needs it -- the only slice consumed by the transport layer
// rather than by another module.
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
