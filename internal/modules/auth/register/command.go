package register

import (
	"context"
	"fmt"

	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

// maxPasswordBytes is bcrypt's hard input limit; inputs longer than this error
// in GenerateFromPassword. validator's max=72 counts runes, so we re-check bytes.
const maxPasswordBytes = 72

type Params struct {
	Email     string
	Password  string
	FirstName string
	LastName  string
}

type Command struct {
	users      UserCreator
	tokens     TokenIssuer
	bcryptCost int
}

func New(users UserCreator, tokens TokenIssuer, bcryptCost int) *Command {
	return &Command{users: users, tokens: tokens, bcryptCost: bcryptCost}
}

func (c *Command) Execute(ctx context.Context, p Params) (*domain.TokenPair, error) {
	if len(p.Password) > maxPasswordBytes {
		return nil, fmt.Errorf("%w: password must not exceed %d bytes", apperror.ErrBadRequest, maxPasswordBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(p.Password), c.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	user, err := c.users.Create(ctx, usercontract.NewUser{
		Email:        p.Email,
		PasswordHash: string(hash),
		FirstName:    p.FirstName,
		LastName:     p.LastName,
	})
	if err != nil {
		return nil, err
	}

	return c.tokens.BuildTokenPair(user)
}
