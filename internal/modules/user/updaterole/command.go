package updaterole

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
)

type Params struct {
	RequesterID uuid.UUID
	TargetID    uuid.UUID
	Role        string
}

type Command struct {
	repo       Repository
	invalidate StatusInvalidator
	logger     *slog.Logger
}

func New(repo Repository, invalidate StatusInvalidator, logger *slog.Logger) *Command {
	return &Command{repo: repo, invalidate: invalidate, logger: logger}
}

func (c *Command) Execute(ctx context.Context, p Params) error {
	if p.RequesterID == p.TargetID {
		return fmt.Errorf("%w: cannot change own role", apperror.ErrForbidden)
	}

	u, err := c.repo.GetByID(ctx, p.TargetID)
	if err != nil {
		return err
	}

	if u.Role == domain.RoleAdmin && p.Role == domain.RoleUser {
		count, err := c.repo.CountAdmins(ctx)
		if err != nil {
			return err
		}
		if count <= 1 {
			return fmt.Errorf("%w: cannot remove last admin", apperror.ErrBadRequest)
		}
	}

	u.Role = p.Role
	if err := c.repo.Update(ctx, u); err != nil {
		return err
	}

	if err := c.repo.IncrementTokenVersion(ctx, p.TargetID); err != nil {
		return fmt.Errorf("revoking tokens after role change: %w", err)
	}

	c.invalidateStatusCache(ctx, p.TargetID)
	return nil
}

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
