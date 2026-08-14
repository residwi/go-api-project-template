package refresh

import (
	"context"

	"github.com/google/uuid"

	authcontract "github.com/residwi/go-api-project-template/internal/modules/auth/contract"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

type UserProvider interface {
	GetByID(ctx context.Context, id uuid.UUID) (usercontract.User, error)
}

type TokenValidator interface {
	ValidateToken(tokenString string) (authcontract.Claims, error)
}

type TokenIssuer interface {
	BuildTokenPair(user usercontract.User) (*domain.TokenPair, error)
}
