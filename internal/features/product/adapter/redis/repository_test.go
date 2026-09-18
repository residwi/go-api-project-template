package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/product"
	"github.com/residwi/go-api-project-template/internal/features/product/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testRedisClient *goredis.Client

func TestMain(m *testing.M) {
	client, cleanup := testutil.MustStartRedis(4)
	defer cleanup()
	testRedisClient = client
	os.Exit(m.Run())
}

func TestRepository_GetBySlug(t *testing.T) {
	t.Run("serves the second call from the cache", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{product: &domain.Product{Slug: "mug", Name: "Mug"}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.GetBySlug(context.Background(), "mug")
		require.NoError(t, err)
		got, err := repo.GetBySlug(context.Background(), "mug")

		require.NoError(t, err)
		assert.Equal(t, &domain.Product{Slug: "mug", Name: "Mug"}, got)
		assert.Equal(t, 1, stub.slugCalls)
	})

	t.Run("caches a not-found slug as absent", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{slugErr: errs.ErrNotFound}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, first := repo.GetBySlug(context.Background(), "ghost")
		_, second := repo.GetBySlug(context.Background(), "ghost")

		require.ErrorIs(t, first, errs.ErrNotFound)
		require.ErrorIs(t, second, errs.ErrNotFound)
		assert.Equal(t, 1, stub.slugCalls)
	})
}

func TestRepository_GetImagesByProductID(t *testing.T) {
	t.Run("serves the second call from the cache", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		id := uuid.New()
		stub := &stubRepo{images: []domain.Image{{ProductID: id, URL: "a.png"}}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.GetImagesByProductID(context.Background(), id)
		require.NoError(t, err)
		got, err := repo.GetImagesByProductID(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, []domain.Image{{ProductID: id, URL: "a.png"}}, got)
		assert.Equal(t, 1, stub.imageCalls)
	})

	t.Run("drops a corrupt entry and reloads", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		id := uuid.New()
		stub := &stubRepo{images: []domain.Image{{ProductID: id, URL: "a.png"}}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())
		require.NoError(t, testRedisClient.Set(
			context.Background(), "product:images:"+id.String(), "{not json", time.Minute).Err())

		got, err := repo.GetImagesByProductID(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, []domain.Image{{ProductID: id, URL: "a.png"}}, got)
		assert.Equal(t, 1, stub.imageCalls)
	})
}

func TestRepository_Invalidation(t *testing.T) {
	t.Run("Delete drops the key held under the product's slug", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		id := uuid.New()
		stub := &stubRepo{product: &domain.Product{ID: id, Slug: "mug", Name: "Mug"}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.GetBySlug(context.Background(), "mug")
		require.NoError(t, err)
		require.NoError(t, repo.Delete(context.Background(), id))

		exists, err := testRedisClient.Exists(context.Background(), "product:slug:mug").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})

	t.Run("Update drops both the old and the new slug", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		id := uuid.New()
		stub := &stubRepo{product: &domain.Product{ID: id, Slug: "mug", Name: "Mug"}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.GetBySlug(context.Background(), "mug")
		require.NoError(t, err)
		require.NoError(t, repo.Update(context.Background(),
			&domain.Product{ID: id, Slug: "tankard", Name: "Tankard"}))

		exists, err := testRedisClient.Exists(
			context.Background(), "product:slug:mug", "product:slug:tankard").Result()
		require.NoError(t, err)
		assert.Equal(t, int64(0), exists)
	})
}

type stubRepo struct {
	product    *domain.Product
	slugErr    error
	slugCalls  int
	images     []domain.Image
	imageCalls int
}

var _ product.Repository = (*stubRepo)(nil)

func (s *stubRepo) GetBySlug(context.Context, string) (*domain.Product, error) {
	s.slugCalls++
	if s.slugErr != nil {
		return nil, s.slugErr
	}

	return s.product, nil
}

func (s *stubRepo) GetByID(context.Context, uuid.UUID) (*domain.Product, error) {
	return s.product, nil
}

func (s *stubRepo) GetImagesByProductID(context.Context, uuid.UUID) ([]domain.Image, error) {
	s.imageCalls++
	return s.images, nil
}

func (s *stubRepo) Update(context.Context, *domain.Product) error { return nil }
func (s *stubRepo) Delete(context.Context, uuid.UUID) error       { return nil }
func (s *stubRepo) AddImage(context.Context, *domain.Image) error { return assert.AnError }
func (s *stubRepo) DeleteImage(context.Context, uuid.UUID) error  { return assert.AnError }

func (s *stubRepo) Create(context.Context, *domain.Product) error {
	return assert.AnError
}

func (s *stubRepo) ListPublished(
	context.Context,
	product.PublishedListParams,
) ([]domain.Product, string, bool, error) {
	return nil, "", false, assert.AnError
}

func (s *stubRepo) ListAdmin(
	context.Context,
	product.AdminListParams,
) ([]domain.Product, int, error) {
	return nil, 0, assert.AnError
}

func (s *stubRepo) GetByIDsIncludingDeleted(context.Context, []uuid.UUID) ([]domain.Product, error) {
	return nil, assert.AnError
}

func (s *stubRepo) CountPublishedByCategory(context.Context, uuid.UUID) (int, error) {
	return 0, assert.AnError
}
