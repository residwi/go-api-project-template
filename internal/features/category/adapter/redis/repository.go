package redis

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/features/category"
	"github.com/residwi/go-api-project-template/internal/features/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
)

const (
	listTTL = 5 * time.Minute
	listKey = "category:list"
)

type Repository struct {
	repo   category.Repository
	rdb    *goredis.Client
	cache  *cache.ReadThrough
	logger *slog.Logger
}

var _ category.Repository = (*Repository)(nil)

func New(repo category.Repository, rdb *goredis.Client, logger *slog.Logger) *Repository {
	return &Repository{
		repo:   repo,
		rdb:    rdb,
		cache:  cache.NewReadThrough(rdb, logger.With(slog.String("cache", "category_list"))),
		logger: logger,
	}
}

func (r *Repository) AncestorDepthAndCycle(
	ctx context.Context,
	parentID, selfID uuid.UUID,
	maxDepth int,
) (int, bool, error) {
	return r.repo.AncestorDepthAndCycle(ctx, parentID, selfID, maxDepth)
}

func (r *Repository) Create(ctx context.Context, cat *domain.Category) error {
	if err := r.repo.Create(ctx, cat); err != nil {
		return err
	}

	r.invalidate(ctx, listKey)

	return nil
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	if err := r.repo.Delete(ctx, id); err != nil {
		return err
	}

	r.invalidate(ctx, listKey)

	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Category, error) {
	return r.repo.GetByID(ctx, id)
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*domain.Category, error) {
	return r.repo.GetBySlug(ctx, slug)
}

func (r *Repository) List(ctx context.Context) ([]domain.Category, error) {
	return r.cache.Take(ctx, listKey, listTTL,
		func(ctx context.Context) ([]domain.Category, error) {
			return r.repo.List(ctx)
		})
}

func (r *Repository) Update(ctx context.Context, cat *domain.Category) error {
	if err := r.repo.Update(ctx, cat); err != nil {
		return err
	}

	r.invalidate(ctx, listKey)

	return nil
}

func (r *Repository) invalidate(ctx context.Context, keys ...string) {
	delCtx := context.WithoutCancel(ctx)

	if err := r.rdb.Del(delCtx, keys...).Err(); err != nil {
		r.logger.WarnContext(delCtx, "failed to invalidate category cache",
			slog.String("error", err.Error()))
	}
}
