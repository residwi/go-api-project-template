package refresh

import (
	"context"

	"github.com/google/uuid"

	authcontract "github.com/residwi/go-api-project-template/internal/modules/auth/contract"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

// UserProvider is satisfied directly by credentials.Store.GetByID -- no adapter.
type UserProvider interface {
	GetByID(ctx context.Context, id uuid.UUID) (usercontract.User, error)
}

// TokenValidator is satisfied directly by token.Service.ValidateToken -- the
// same shape middleware.TokenValidator needs, so both consumers wire to the
// same method, no adapter either place.
type TokenValidator interface {
	ValidateToken(tokenString string) (authcontract.Claims, error)
}

// TokenIssuer is satisfied directly by token.Service.BuildTokenPair -- no
// adapter.
type TokenIssuer interface {
	BuildTokenPair(user usercontract.User) (*domain.TokenPair, error)
}
