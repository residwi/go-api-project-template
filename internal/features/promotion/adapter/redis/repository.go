package redis

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/features/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/cache"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const (
	codeTTL      = 30 * time.Second
	writeTimeout = time.Second
)

type Repository struct {
	repo   promotion.Repository
	rdb    *goredis.Client
	cache  *cache.ReadThrough
	logger *slog.Logger
}

var _ promotion.Repository = (*Repository)(nil)

func New(repo promotion.Repository, rdb *goredis.Client, logger *slog.Logger) *Repository {
	return &Repository{
		repo:   repo,
		rdb:    rdb,
		cache:  cache.NewReadThrough(rdb, logger.With(slog.String("cache", "promotion_code"))),
		logger: logger,
	}
}

func (r *Repository) ApplyPromotion(ctx context.Context, id uuid.UUID) error {
	return r.repo.ApplyPromotion(ctx, id)
}

func (r *Repository) Create(ctx context.Context, promo *domain.Promotion) error {
	return r.repo.Create(ctx, promo)
}

func (r *Repository) CreateUsage(ctx context.Context, usage *domain.CouponUsage) error {
	return r.repo.CreateUsage(ctx, usage)
}

func (r *Repository) Delete(ctx context.Context, id uuid.UUID) error {
	previous, err := r.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	if err := r.repo.Delete(ctx, id); err != nil {
		return err
	}

	r.invalidate(ctx, codeKey(previous.Code))

	return nil
}

func (r *Repository) DeleteUsageByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.CouponUsage, error) {
	return r.repo.DeleteUsageByOrderID(ctx, orderID)
}

func (r *Repository) GetByCode(ctx context.Context, code string) (*domain.Promotion, error) {
	// Reserve reads this inside its transaction, prices the order from Value, then
	// increments used_count. ApplyPromotion's WHERE guards the count but never
	// re-checks Value, so a cached row here would write a stale discount to the order.
	if database.InTx(ctx) {
		return r.repo.GetByCode(ctx, code)
	}

	return r.cache.Take(ctx, codeKey(code), codeTTL,
		func(ctx context.Context) (*domain.Promotion, error) {
			return r.repo.GetByCode(ctx, code)
		})
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Promotion, error) {
	return r.repo.GetByID(ctx, id)
}

func (r *Repository) ListAdmin(
	ctx context.Context,
	params promotion.AdminListParams,
) ([]domain.Promotion, int, error) {
	return r.repo.ListAdmin(ctx, params)
}

func (r *Repository) ReleasePromotion(ctx context.Context, id uuid.UUID) error {
	return r.repo.ReleasePromotion(ctx, id)
}

func (r *Repository) Update(ctx context.Context, promo *domain.Promotion) error {
	previous, err := r.repo.GetByID(ctx, promo.ID)
	if err != nil {
		return err
	}

	if err := r.repo.Update(ctx, promo); err != nil {
		return err
	}

	r.invalidate(ctx, codeKey(previous.Code), codeKey(promo.Code))

	return nil
}

func (r *Repository) invalidate(ctx context.Context, keys ...string) {
	delCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), writeTimeout)
	defer cancel()

	if err := r.rdb.Del(delCtx, keys...).Err(); err != nil {
		r.logger.WarnContext(delCtx, "failed to invalidate promotion cache",
			slog.String("error", err.Error()))
	}
}

func codeKey(code string) string { return "promotion:code:" + code }
