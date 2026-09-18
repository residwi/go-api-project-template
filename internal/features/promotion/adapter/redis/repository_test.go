package redis

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/features/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var (
	testRedisClient *goredis.Client
	testPool        *pgxpool.Pool
)

func TestMain(m *testing.M) {
	pool, cleanupPostgres := testutil.MustStartPostgres("test_promotion_redis")
	defer cleanupPostgres()
	testPool = pool

	client, cleanupRedis := testutil.MustStartRedis(6)
	defer cleanupRedis()
	testRedisClient = client

	os.Exit(m.Run())
}

func TestRepository_GetByCode(t *testing.T) {
	t.Run("serves the second call from the cache", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{promo: &domain.Promotion{Code: "SAVE10", Value: 10}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.GetByCode(context.Background(), "SAVE10")
		require.NoError(t, err)
		got, err := repo.GetByCode(context.Background(), "SAVE10")

		require.NoError(t, err)
		assert.Equal(t, &domain.Promotion{Code: "SAVE10", Value: 10}, got)
		assert.Equal(t, 1, stub.codeCalls)
	})

	t.Run("caches an unknown code as absent", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{codeErr: errs.ErrNotFound}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, first := repo.GetByCode(context.Background(), "NOPE")
		_, second := repo.GetByCode(context.Background(), "NOPE")

		require.ErrorIs(t, first, errs.ErrNotFound)
		require.ErrorIs(t, second, errs.ErrNotFound)
		assert.Equal(t, 1, stub.codeCalls)
	})

	t.Run("reads through inside a transaction and does not populate the cache", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{promo: &domain.Promotion{Code: "SAVE10", Value: 10}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		err := database.WithTx(context.Background(), testPool, func(txCtx context.Context) error {
			if _, txErr := repo.GetByCode(txCtx, "SAVE10"); txErr != nil {
				return txErr
			}
			_, txErr := repo.GetByCode(txCtx, "SAVE10")
			return txErr
		})
		require.NoError(t, err)

		assert.Equal(t, 2, stub.codeCalls)
		exists, err := testRedisClient.Exists(context.Background(), "promotion:code:SAVE10").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})
}

func TestRepository_Invalidation(t *testing.T) {
	t.Run("Delete drops the key held under the promotion's code", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		id := uuid.New()
		stub := &stubRepo{promo: &domain.Promotion{ID: id, Code: "SAVE10", Value: 10}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.GetByCode(context.Background(), "SAVE10")
		require.NoError(t, err)
		require.NoError(t, repo.Delete(context.Background(), id))

		exists, err := testRedisClient.Exists(context.Background(), "promotion:code:SAVE10").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})

	t.Run("Update drops both the old and the new code", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		id := uuid.New()
		stub := &stubRepo{promo: &domain.Promotion{ID: id, Code: "SAVE10", Value: 10}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.GetByCode(context.Background(), "SAVE10")
		require.NoError(t, err)
		require.NoError(t, repo.Update(context.Background(),
			&domain.Promotion{ID: id, Code: "SAVE20", Value: 20}))

		exists, err := testRedisClient.Exists(
			context.Background(), "promotion:code:SAVE10", "promotion:code:SAVE20").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})
}

type stubRepo struct {
	promo     *domain.Promotion
	codeErr   error
	codeCalls int
}

var _ promotion.Repository = (*stubRepo)(nil)

func (s *stubRepo) GetByCode(context.Context, string) (*domain.Promotion, error) {
	s.codeCalls++
	if s.codeErr != nil {
		return nil, s.codeErr
	}

	return s.promo, nil
}

func (s *stubRepo) GetByID(context.Context, uuid.UUID) (*domain.Promotion, error) {
	return s.promo, nil
}

func (s *stubRepo) Create(context.Context, *domain.Promotion) error { return assert.AnError }
func (s *stubRepo) Update(context.Context, *domain.Promotion) error { return nil }
func (s *stubRepo) Delete(context.Context, uuid.UUID) error         { return nil }

func (s *stubRepo) ListAdmin(
	context.Context,
	promotion.AdminListParams,
) ([]domain.Promotion, int, error) {
	return nil, 0, assert.AnError
}

func (s *stubRepo) ApplyPromotion(context.Context, uuid.UUID) error   { return assert.AnError }
func (s *stubRepo) ReleasePromotion(context.Context, uuid.UUID) error { return assert.AnError }
func (s *stubRepo) CreateUsage(context.Context, *domain.CouponUsage) error {
	return assert.AnError
}

func (s *stubRepo) DeleteUsageByOrderID(context.Context, uuid.UUID) (*domain.CouponUsage, error) {
	return nil, assert.AnError
}
