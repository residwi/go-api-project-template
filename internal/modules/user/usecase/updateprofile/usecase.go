package updateprofile

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
)

type Params struct {
	FirstName string
	LastName  string
	Phone     *string
}

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (c *UseCase) Execute(ctx context.Context, id uuid.UUID, p Params) (*domain.User, error) {
	u, err := c.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	if p.FirstName != "" {
		u.FirstName = p.FirstName
	}
	if p.LastName != "" {
		u.LastName = p.LastName
	}
	if p.Phone != nil {
		u.Phone = *p.Phone
	}

	if err := c.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	return u, nil
}
