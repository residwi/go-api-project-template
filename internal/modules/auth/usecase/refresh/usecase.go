package refresh

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
)

type UseCase struct {
	users    UserProvider
	validate TokenValidator
	tokens   TokenIssuer
}

func New(users UserProvider, validate TokenValidator, tokens TokenIssuer) *UseCase {
	return &UseCase{users: users, validate: validate, tokens: tokens}
}

func (c *UseCase) Execute(ctx context.Context, refreshToken string) (*domain.TokenPair, error) {
	claims, err := c.validate.ValidateToken(refreshToken)
	if err != nil {
		return nil, apperror.ErrInvalidToken
	}

	if claims.Type != "refresh" {
		return nil, apperror.ErrInvalidToken
	}

	user, err := c.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}

	if !user.Active {
		return nil, apperror.ErrUnauthorized
	}

	if user.TokenVersion != claims.TokenVersion {
		return nil, apperror.ErrInvalidToken
	}

	return c.tokens.BuildTokenPair(user)
}
