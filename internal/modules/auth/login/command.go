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

type Command struct {
	users     UserProvider
	tokens    TokenIssuer
	dummyHash []byte
}

// New hashes dummyHash at bcryptCost, which must match register's hashing
// cost -- otherwise the dummy comparison in Execute runs at a different
// speed than a real one and reopens the timing oracle it exists to close.
func New(users UserProvider, tokens TokenIssuer, bcryptCost int) *Command {
	c := &Command{users: users, tokens: tokens}
	c.dummyHash, _ = bcrypt.GenerateFromPassword([]byte(dummyPassword), bcryptCost)
	return c
}

func (c *Command) Execute(ctx context.Context, p Params) (*domain.TokenPair, error) {
	creds, err := c.users.GetByEmail(ctx, p.Email)
	if err != nil {
		// Run a dummy comparison so an unknown email takes about as long as a
		// wrong password, removing the timing oracle for account enumeration.
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
