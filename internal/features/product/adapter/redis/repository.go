package redis

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/features/product"
	"github.com/residwi/go-api-project-template/internal/features/product/domain"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
)

const (
	slugTTL      = 60 * time.Second
	imagesTTL    = 60 * time.Second
	writeTimeout = time.Second
)

type Repository struct {
	repo   product.Repository
	rdb    *goredis.Client
	cache  *cache.ReadThrough
	logger *slog.Logger
}

var _ product.Repository = (*Repository)(nil)

func New(repo product.Repository, rdb *goredis.Client, logger *slog.Logger) *Repository {
	return &Repository{
		repo:   repo,
		rdb:    rdb,
		cache:  cache.NewReadThrough(rdb, logger.With(slog.String("cache", "product_slug"))),
		logger: logger,
	}
}

func (r *Repository) AddImage(ctx context.Context, img *domain.Image) error {
	if err := r.repo.AddImage(ctx, img); err != nil {
		return err
	}

	r.invalidate(ctx, imagesKey(img.ProductID))

	return nil
}

func (r *Repository) CountPublishedByCategory(ctx context.Context, categoryID uuid.UUID) (int, error) {
	return r.repo.CountPublishedByCategory(ctx, categoryID)
}

func (r *Repository) Create(ctx context.Context, p *domain.Product) error {
	return r.repo.Create(ctx, p)
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	previous, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := r.repo.Delete(ctx, id); err != nil {
		return err
	}

	r.invalidate(ctx, slugKey(previous.Slug), imagesKey(id))

	return nil
}

func (r *Repository) DeleteImage(ctx context.Context, imageID uuid.UUID) error {
	return r.repo.DeleteImage(ctx, imageID)
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Product, error) {
	return r.repo.GetByID(ctx, id)
}

func (r *Repository) GetByIDsIncludingDeleted(ctx context.Context, ids []uuid.UUID) ([]domain.Product, error) {
	return r.repo.GetByIDsIncludingDeleted(ctx, ids)
}

func (r *Repository) GetBySlug(ctx context.Context, slug string) (*domain.Product, error) {
	return r.cache.Take(ctx, slugKey(slug), slugTTL,
		func(ctx context.Context) (*domain.Product, error) {
			return r.repo.GetBySlug(ctx, slug)
		})
}

func (r *Repository) GetImagesByProductID(ctx context.Context, productID uuid.UUID) ([]domain.Image, error) {
	key := imagesKey(productID)

	raw, err := r.rdb.Get(ctx, key).Bytes()
	switch {
	case err == nil:
		var images []domain.Image
		//nolint:musttag // cache codec only, no wire shape: domain types carry no json tags by convention
		if unmarshalErr := json.Unmarshal(raw, &images); unmarshalErr == nil {
			return images, nil
		}

		r.logger.WarnContext(ctx, "corrupt cache entry, dropping", slog.String("key", key))
		r.invalidate(ctx, key)
	case !errors.Is(err, goredis.Nil):
		r.logger.WarnContext(ctx, "cache read failed, falling back to loader",
			slog.String("key", key), slog.String("error", err.Error()))
	}

	images, err := r.repo.GetImagesByProductID(ctx, productID)
	if err != nil {
		return nil, err
	}

	r.put(ctx, key, images, imagesTTL)

	return images, nil
}

func (r *Repository) ListAdmin(
	ctx context.Context,
	params product.AdminListParams,
) ([]domain.Product, int, error) {
	return r.repo.ListAdmin(ctx, params)
}

func (r *Repository) ListPublished(
	ctx context.Context,
	params product.PublishedListParams,
) ([]domain.Product, string, bool, error) {
	return r.repo.ListPublished(ctx, params)
}

func (r *Repository) Update(ctx context.Context, p *domain.Product) error {
	previous, err := r.repo.GetByID(ctx, p.ID)
	if err != nil {
		return err
	}

	if err := r.repo.Update(ctx, p); err != nil {
		return err
	}

	r.invalidate(ctx, slugKey(previous.Slug), slugKey(p.Slug))

	return nil
}

func (r *Repository) put(ctx context.Context, key string, value any, ttl time.Duration) {
	data, err := json.Marshal(value)
	if err != nil {
		r.logger.WarnContext(ctx, "cache encode failed",
			slog.String("key", key), slog.String("error", err.Error()))
		return
	}

	writeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()

	if err := r.rdb.Set(writeCtx, key, data, ttl).Err(); err != nil {
		r.logger.WarnContext(writeCtx, "cache write failed",
			slog.String("key", key), slog.String("error", err.Error()))
	}
}

func (r *Repository) invalidate(ctx context.Context, keys ...string) {
	delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()

	if err := r.rdb.Del(delCtx, keys...).Err(); err != nil {
		r.logger.WarnContext(delCtx, "failed to invalidate product cache",
			slog.String("error", err.Error()))
	}
}

func imagesKey(productID uuid.UUID) string { return "product:images:" + productID.String() }

func slugKey(slug string) string { return "product:slug:" + slug }
