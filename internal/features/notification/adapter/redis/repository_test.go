package redis

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/features/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testRedisClient *goredis.Client

func TestMain(m *testing.M) {
	client, cleanup := testutil.MustStartRedis(7)
	defer cleanup()
	testRedisClient = client
	os.Exit(m.Run())
}

func TestRepository_CountUnread(t *testing.T) {
	t.Run("counts from the wrapped repository on a miss", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		userID := uuid.New()
		stub := &stubRepo{unread: 3}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		got, err := repo.CountUnread(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, 3, got)
		assert.Equal(t, 1, stub.countCalls)
	})

	t.Run("serves the second call from Redis", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		userID := uuid.New()
		stub := &stubRepo{unread: 3}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		got, err := repo.CountUnread(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, 3, got)
		assert.Equal(t, 1, stub.countCalls)
	})

	t.Run("Create increments without recounting", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		userID := uuid.New()
		stub := &stubRepo{unread: 3}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		require.NoError(t, repo.Create(context.Background(), &domain.Notification{UserID: userID}))
		got, err := repo.CountUnread(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, 4, got)
		assert.Equal(t, 1, stub.countCalls)
	})

	t.Run("MarkRead decrements", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		userID := uuid.New()
		stub := &stubRepo{unread: 3}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		require.NoError(t, repo.MarkRead(context.Background(), userID, uuid.New()))
		got, err := repo.CountUnread(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, 2, got)
	})

	t.Run("MarkAllRead zeroes the counter", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		userID := uuid.New()
		stub := &stubRepo{unread: 3}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		require.NoError(t, repo.MarkAllRead(context.Background(), userID))
		got, err := repo.CountUnread(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, 0, got)
	})

	t.Run("a failed write leaves the counter alone", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		userID := uuid.New()
		stub := &stubRepo{unread: 3, createErr: assert.AnError}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		require.Error(t, repo.Create(context.Background(), &domain.Notification{UserID: userID}))
		got, err := repo.CountUnread(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, 3, got)
	})

	t.Run("MarkRead below zero floors the counter instead of going negative", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		userID := uuid.New()
		stub := &stubRepo{unread: 1}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		require.NoError(t, repo.MarkRead(context.Background(), userID, uuid.New()))
		require.NoError(t, repo.MarkRead(context.Background(), userID, uuid.New()))
		got, err := repo.CountUnread(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, 0, got)
		assert.Equal(t, 1, stub.countCalls)
	})

	t.Run("recounts after the key expires", func(t *testing.T) {
		require.NoError(t, testRedisClient.FlushDB(context.Background()).Err())
		userID := uuid.New()
		stub := &stubRepo{unread: 3}
		repo := New(stub, testRedisClient, testutil.DiscardLogger())

		_, err := repo.CountUnread(context.Background(), userID)
		require.NoError(t, err)
		require.NoError(t, testRedisClient.Del(
			context.Background(), "notification:unread:"+userID.String()).Err())
		got, err := repo.CountUnread(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, 3, got)
		assert.Equal(t, 2, stub.countCalls)
	})
}

type stubRepo struct {
	unread     int
	countCalls int
	createErr  error
}

var _ notification.Repository = (*stubRepo)(nil)

func (s *stubRepo) Create(context.Context, *domain.Notification) error { return s.createErr }

func (s *stubRepo) Get(context.Context, uuid.UUID) (domain.Notification, error) {
	return domain.Notification{}, assert.AnError
}

func (s *stubRepo) ListByUser(
	context.Context,
	uuid.UUID,
	paging.CursorPage,
) ([]domain.Notification, error) {
	return nil, assert.AnError
}

func (s *stubRepo) CountUnread(context.Context, uuid.UUID) (int, error) {
	s.countCalls++
	return s.unread, nil
}

func (s *stubRepo) MarkRead(context.Context, uuid.UUID, uuid.UUID) error { return nil }

func (s *stubRepo) MarkAllRead(context.Context, uuid.UUID) error { return nil }
