package redis

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

// This package owns Redis DB index 6; see the registry in internal/testutil.
var testRedis *goredis.Client

func TestMain(m *testing.M) {
	rdb, cleanup := testutil.MustStartRedis(6)
	defer cleanup()
	testRedis = rdb
	os.Exit(m.Run())
}

func TestRepository_GetStatusByID(t *testing.T) {
	t.Run("serves the second read from the cache", func(t *testing.T) {
		ctx := context.Background()
		stub := &stubRepository{active: true, tokenVersion: 4}
		r := New(stub, testRedis, testutil.DiscardLogger())
		id := uuid.New()

		active, version, err := r.GetStatusByID(ctx, id)
		require.NoError(t, err)
		assert.True(t, active)
		assert.Equal(t, 4, version)

		active, version, err = r.GetStatusByID(ctx, id)

		require.NoError(t, err)
		assert.True(t, active)
		assert.Equal(t, 4, version)
		assert.Equal(t, int64(1), stub.statusCalls.Load(), "the second read must not reach the repository")
	})

	t.Run("caches a not-found and does not re-query", func(t *testing.T) {
		ctx := context.Background()
		stub := &stubRepository{statusErr: errs.ErrNotFound}
		r := New(stub, testRedis, testutil.DiscardLogger())
		id := uuid.New()

		_, _, err := r.GetStatusByID(ctx, id)
		require.ErrorIs(t, err, errs.ErrNotFound)

		_, _, err = r.GetStatusByID(ctx, id)

		require.ErrorIs(t, err, errs.ErrNotFound)
		assert.Equal(t, int64(1), stub.statusCalls.Load(), "absence must be cached")
	})
}

func TestRepository_Invalidation(t *testing.T) {
	t.Run("Update drops the cached status", func(t *testing.T) {
		ctx := context.Background()
		stub := &stubRepository{active: true, tokenVersion: 1}
		r := New(stub, testRedis, testutil.DiscardLogger())
		id := uuid.New()

		_, _, err := r.GetStatusByID(ctx, id)
		require.NoError(t, err)
		require.NoError(t, r.Update(ctx, &domain.User{ID: id}))
		assert.Equal(t, int64(1), stub.updateCalls.Load(), "Update must delegate to the wrapped repository")

		_, _, err = r.GetStatusByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, int64(2), stub.statusCalls.Load(), "the read after Update must reach the repository")
	})

	t.Run("Delete drops the cached status", func(t *testing.T) {
		ctx := context.Background()
		stub := &stubRepository{active: true, tokenVersion: 1}
		r := New(stub, testRedis, testutil.DiscardLogger())
		id := uuid.New()

		_, _, err := r.GetStatusByID(ctx, id)
		require.NoError(t, err)
		require.NoError(t, r.Delete(ctx, id))
		assert.Equal(t, int64(1), stub.deleteCalls.Load(), "Delete must delegate to the wrapped repository")

		_, _, err = r.GetStatusByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, int64(2), stub.statusCalls.Load())
	})

	t.Run("IncrementTokenVersion drops the cached status", func(t *testing.T) {
		ctx := context.Background()
		stub := &stubRepository{active: true, tokenVersion: 1}
		r := New(stub, testRedis, testutil.DiscardLogger())
		id := uuid.New()

		_, _, err := r.GetStatusByID(ctx, id)
		require.NoError(t, err)
		require.NoError(t, r.IncrementTokenVersion(ctx, id))
		assert.Equal(
			t,
			int64(1),
			stub.bumpCalls.Load(),
			"IncrementTokenVersion must delegate to the wrapped repository",
		)

		_, _, err = r.GetStatusByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, int64(2), stub.statusCalls.Load())
	})

	t.Run("invalidates even when the caller's context is already cancelled", func(t *testing.T) {
		stub := &stubRepository{active: true, tokenVersion: 1}
		r := New(stub, testRedis, testutil.DiscardLogger())
		id := uuid.New()

		_, _, err := r.GetStatusByID(context.Background(), id)
		require.NoError(t, err)

		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		require.NoError(t, r.Delete(cancelled, id))

		_, _, err = r.GetStatusByID(context.Background(), id)

		require.NoError(t, err)
		assert.Equal(t, int64(2), stub.statusCalls.Load(), "a cancelled request must still purge the cache")
	})

	t.Run("does not invalidate when the wrapped write fails", func(t *testing.T) {
		ctx := context.Background()
		stub := &stubRepository{active: true, tokenVersion: 1, writeErr: errors.New("update failed")}
		r := New(stub, testRedis, testutil.DiscardLogger())
		id := uuid.New()

		_, _, err := r.GetStatusByID(ctx, id)
		require.NoError(t, err)

		err = r.Update(ctx, &domain.User{ID: id})
		require.ErrorIs(t, err, stub.writeErr)

		_, _, err = r.GetStatusByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, int64(1), stub.statusCalls.Load(), "a failed write must not purge the cache")
	})
}

func TestRepository_PassThrough(t *testing.T) {
	t.Run("delegates the uncached methods to the wrapped repository", func(t *testing.T) {
		ctx := context.Background()
		stub := &stubRepository{adminCount: 3}
		r := New(stub, testRedis, testutil.DiscardLogger())

		require.NoError(t, r.Create(ctx, &domain.User{ID: uuid.New()}))

		gotByID, err := r.GetByID(ctx, uuid.New())
		require.NoError(t, err)
		assert.NotNil(t, gotByID)

		gotByEmail, err := r.GetByEmail(ctx, "a@b.c")
		require.NoError(t, err)
		assert.NotNil(t, gotByEmail)

		list, total, err := r.ListAdmin(ctx, user.AdminListParams{})
		require.NoError(t, err)
		assert.Nil(t, list)
		assert.Zero(t, total)

		count, err := r.CountAdmins(ctx)
		require.NoError(t, err)
		assert.Equal(t, 3, count)

		assert.Equal(t, int64(1), stub.createCalls.Load())
	})
}

type stubRepository struct {
	active       bool
	tokenVersion int
	statusErr    error
	writeErr     error
	statusCalls  atomic.Int64
	updateCalls  atomic.Int64
	deleteCalls  atomic.Int64
	bumpCalls    atomic.Int64
	createCalls  atomic.Int64
	adminCount   int
}

func (s *stubRepository) GetStatusByID(context.Context, uuid.UUID) (bool, int, error) {
	s.statusCalls.Add(1)
	if s.statusErr != nil {
		return false, 0, s.statusErr
	}

	return s.active, s.tokenVersion, nil
}

func (s *stubRepository) Update(context.Context, *domain.User) error {
	s.updateCalls.Add(1)
	return s.writeErr
}

func (s *stubRepository) Delete(context.Context, uuid.UUID) error {
	s.deleteCalls.Add(1)
	return s.writeErr
}

func (s *stubRepository) IncrementTokenVersion(context.Context, uuid.UUID) error {
	s.bumpCalls.Add(1)
	return s.writeErr
}

func (s *stubRepository) Create(context.Context, *domain.User) error {
	s.createCalls.Add(1)
	return nil
}

func (s *stubRepository) GetByID(context.Context, uuid.UUID) (*domain.User, error) {
	return &domain.User{}, nil
}

func (s *stubRepository) GetByEmail(context.Context, string) (*domain.User, error) {
	return &domain.User{}, nil
}

func (s *stubRepository) ListAdmin(context.Context, user.AdminListParams) ([]domain.User, int, error) {
	return nil, 0, nil
}

func (s *stubRepository) CountAdmins(context.Context) (int, error) {
	return s.adminCount, nil
}

var _ user.Repository = (*stubRepository)(nil)
