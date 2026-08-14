package login

import (
	"context"

	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

// dummyPassword is hashed once per cost to give the unknown-email login path
// roughly the same latency as a real bcrypt comparison.
const dummyPassword = "invalid-user-timing-equalizer"

type Params struct {
	Email    string
	Password string
}

type UseCase struct {
	users     UserProvider
	tokens    TokenIssuer
	dummyHash []byte
}

func New(users UserProvider, tokens TokenIssuer, bcryptCost int) *UseCase {
	c := &UseCase{users: users, tokens: tokens}
	c.dummyHash, _ = bcrypt.GenerateFromPassword([]byte(dummyPassword), bcryptCost)
	return c
}

func (c *UseCase) Execute(ctx context.Context, p Params) (*domain.TokenPair, error) {
	creds, err := c.users.GetByEmail(ctx, p.Email)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(c.dummyHash, []byte(p.Password))
		return nil, apperror.ErrInvalidCredentials
	}

	if !creds.Active {
		return nil, apperror.ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(p.Password)); err != nil {
		return nil, apperror.ErrInvalidCredentials
	}

	return c.tokens.BuildTokenPair(usercontract.User{
		ID:           creds.ID,
		Email:        creds.Email,
		FirstName:    creds.FirstName,
		LastName:     creds.LastName,
		Role:         creds.Role,
		Active:       creds.Active,
		TokenVersion: creds.TokenVersion,
	})
}
