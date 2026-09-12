package redis

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
)

const (
	statusTTL         = 30 * time.Second
	statusAbsentTTL   = 5 * time.Second
	statusDeviation   = 0.1
	statusLoadTimeout = time.Second
	invalidateTimeout = time.Second
)

type Repository struct {
	repo   user.Repository
	rdb    *goredis.Client
	cache  *cache.ReadThrough
	logger *slog.Logger
}

var _ user.Repository = (*Repository)(nil)

func New(repo user.Repository, rdb *goredis.Client, logger *slog.Logger) *Repository {
	return &Repository{
		repo:   repo,
		rdb:    rdb,
		cache:  cache.NewReadThrough(rdb, logger.With(slog.String("cache", "user_status"))),
		logger: logger,
	}
}

func (r *Repository) GetStatusByID(ctx context.Context, id uuid.UUID) (bool, int, error) {
	status, err := r.cache.Take(
		ctx,
		statusOptions(),
		statusKey(id),
		func(ctx context.Context) (user.AccountStatus, error) {
			active, tokenVersion, loadErr := r.repo.GetStatusByID(ctx, id)
			if loadErr != nil {
				return user.AccountStatus{}, loadErr
			}

			return user.AccountStatus{Active: active, TokenVersion: tokenVersion}, nil
		},
	)
	if err != nil {
		return false, 0, err
	}

	return status.Active, status.TokenVersion, nil
}

func (r *Repository) Update(ctx context.Context, u *domain.User) error {
	if err := r.repo.Update(ctx, u); err != nil {
		return err
	}

	r.invalidate(ctx, u.ID)

	return nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.repo.Delete(ctx, id); err != nil {
		return err
	}

	r.invalidate(ctx, id)

	return nil
}

func (r *Repository) IncrementTokenVersion(ctx context.Context, id uuid.UUID) error {
	if err := r.repo.IncrementTokenVersion(ctx, id); err != nil {
		return err
	}

	r.invalidate(ctx, id)

	return nil
}

func (r *Repository) Create(ctx context.Context, u *domain.User) error {
	return r.repo.Create(ctx, u)
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	return r.repo.GetByID(ctx, id)
}

func (r *Repository) GetByEmail(ctx context.Context, email string) (*domain.User, error) {
	return r.repo.GetByEmail(ctx, email)
}

func (r *Repository) ListAdmin(ctx context.Context, params user.AdminListParams) ([]domain.User, int, error) {
	return r.repo.ListAdmin(ctx, params)
}

func (r *Repository) CountAdmins(ctx context.Context) (int, error) {
	return r.repo.CountAdmins(ctx)
}

func (r *Repository) invalidate(ctx context.Context, id uuid.UUID) {
	delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), invalidateTimeout)
	defer cancel()

	if err := r.rdb.Del(delCtx, statusKey(id)).Err(); err != nil {
		r.logger.WarnContext(delCtx, "failed to invalidate user status cache",
			slog.String("target_user_id", id.String()), slog.String("error", err.Error()))
	}
}

func statusOptions() cache.Options {
	return cache.Options{
		TTL:         statusTTL,
		AbsentTTL:   statusAbsentTTL,
		Deviation:   statusDeviation,
		LoadTimeout: statusLoadTimeout,
	}
}

func statusKey(id uuid.UUID) string { return "user:status:v2:" + id.String() }
