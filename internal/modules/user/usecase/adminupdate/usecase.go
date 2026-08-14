package adminupdate

import (
	"context"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
)

type Params struct {
	FirstName string
	LastName  string
	Phone     *string
	Active    *bool
}

type UseCase struct {
	repo       Repository
	invalidate StatusInvalidator
	logger     *slog.Logger
}

func New(repo Repository, invalidate StatusInvalidator, logger *slog.Logger) *UseCase {
	return &UseCase{repo: repo, invalidate: invalidate, logger: logger}
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
	if p.Active != nil {
		u.Active = *p.Active
	}

	if err := c.repo.Update(ctx, u); err != nil {
		return nil, err
	}

	c.invalidateStatusCache(ctx, id)

	return u, nil
}

func (c *UseCase) invalidateStatusCache(ctx context.Context, userID uuid.UUID) {
	if err := c.invalidate.Invalidate(ctx, userID); err != nil {
		c.logger.WarnContext(
			ctx,
			"failed to invalidate user status cache",
			slog.String("target_user_id", userID.String()),
			slog.String("error", err.Error()),
		)
	}
}
