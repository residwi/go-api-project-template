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

// Command takes no TxRunner: it loads one row through its own repository,
// patches it and writes it back, with nothing else to ask.
type Command struct {
	repo       Repository
	invalidate StatusInvalidator
	logger     *slog.Logger
}

func New(repo Repository, invalidate StatusInvalidator, logger *slog.Logger) *Command {
	return &Command{repo: repo, invalidate: invalidate, logger: logger}
}

func (c *Command) Execute(ctx context.Context, id uuid.UUID, p Params) (*domain.User, error) {
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

// Without this a revoked or deactivated user keeps access until the TTL lapses.
// Best-effort: a failure is logged and the entry still expires on its own.
func (c *Command) invalidateStatusCache(ctx context.Context, userID uuid.UUID) {
	if err := c.invalidate.Invalidate(ctx, userID); err != nil {
		c.logger.WarnContext(
			ctx,
			"failed to invalidate user status cache",
			slog.String("target_user_id", userID.String()),
			slog.String("error", err.Error()),
		)
	}
}
