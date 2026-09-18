package redis

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/category"
	"github.com/residwi/go-api-project-template/internal/features/category/domain"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testRedisClient *goredis.Client

func TestMain(m *testing.M) {
	client, cleanup := testutil.MustStartRedis(2)
	defer cleanup()
	testRedisClient = client
	os.Exit(m.Run())
}

func TestRepository_List(t *testing.T) {
	t.Run("loads from the wrapped repository on a miss", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{categories: []domain.Category{{Name: "Books"}}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		got, err := repo.List(context.Background())

		require.NoError(t, err)
		assert.Equal(t, []domain.Category{{Name: "Books"}}, got)
		assert.Equal(t, 1, stub.listCalls)
	})

	t.Run("serves the second call from the cache", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{categories: []domain.Category{{Name: "Books"}}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.List(context.Background())
		require.NoError(t, err)
		got, err := repo.List(context.Background())

		require.NoError(t, err)
		assert.Equal(t, []domain.Category{{Name: "Books"}}, got)
		assert.Equal(t, 1, stub.listCalls)
	})

	t.Run("collapses concurrent callers into one load", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{
			categories: []domain.Category{{Name: "Books"}},
			block:      make(chan struct{}),
			firstCall:  make(chan struct{}),
		}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		const callers = 5
		done := make(chan error, callers)
		for range callers {
			go func() {
				_, err := repo.List(context.Background())
				done <- err
			}()
		}

		stub.waitForFirstCall(t)
		close(stub.block)
		for range callers {
			require.NoError(t, <-done)
		}

		assert.Equal(t, 1, stub.listCalls)
	})
}

func TestRepository_Invalidation(t *testing.T) {
	t.Run("Create drops the cached list", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{categories: []domain.Category{{Name: "Books"}}}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.List(context.Background())
		require.NoError(t, err)
		require.NoError(t, repo.Create(context.Background(), &domain.Category{Name: "Toys"}))
		_, err = repo.List(context.Background())

		require.NoError(t, err)
		assert.Equal(t, 2, stub.listCalls)
	})

	t.Run("a failed write leaves the cache alone", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		stub := &stubRepo{
			categories: []domain.Category{{Name: "Books"}},
			createErr:  assert.AnError,
		}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.List(context.Background())
		require.NoError(t, err)
		require.Error(t, repo.Create(context.Background(), &domain.Category{Name: "Toys"}))
		_, err = repo.List(context.Background())

		require.NoError(t, err)
		assert.Equal(t, 1, stub.listCalls)
	})
}

type stubRepo struct {
	categories []domain.Category
	listCalls  int
	createErr  error
	block      chan struct{}
	firstCall  chan struct{}
	mu         sync.Mutex
}

var _ category.Repository = (*stubRepo)(nil)

func (s *stubRepo) List(context.Context) ([]domain.Category, error) {
	s.mu.Lock()
	s.listCalls++
	if s.firstCall != nil && s.listCalls == 1 {
		close(s.firstCall)
	}
	s.mu.Unlock()

	if s.block != nil {
		<-s.block
	}

	return s.categories, nil
}

func (s *stubRepo) Create(context.Context, *domain.Category) error { return s.createErr }
func (s *stubRepo) GetByID(context.Context, uuid.UUID) (*domain.Category, error) {
	return nil, assert.AnError
}

func (s *stubRepo) GetBySlug(context.Context, string) (*domain.Category, error) {
	return nil, assert.AnError
}
func (s *stubRepo) Update(context.Context, *domain.Category) error { return nil }
func (s *stubRepo) Delete(context.Context, uuid.UUID) error        { return nil }
func (s *stubRepo) AncestorDepthAndCycle(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	int,
) (int, bool, error) {
	return 0, false, nil
}

func (s *stubRepo) waitForFirstCall(t *testing.T) {
	t.Helper()
	select {
	case <-s.firstCall:
	case <-time.After(2 * time.Second):
		t.Fatal("wrapped repository was never called")
	}
}
