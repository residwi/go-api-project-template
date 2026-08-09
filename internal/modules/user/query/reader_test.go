package query

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user/contract"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestReader_CheckStatus(t *testing.T) {
	t.Parallel()

	userID := uuid.New()

	t.Run("returns the cached snapshot without touching the repository", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).
			Return(StatusSnapshot{Active: true, TokenVersion: 42}, true, nil)
		r := New(repo, c, testhelper.DiscardLogger())

		got, err := r.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, contract.AccountStatus{Active: true, TokenVersion: 42}, got)
	})

	t.Run("reads the repository on a miss and writes the snapshot back", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(true, 7, nil)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).Return(StatusSnapshot{}, false, nil)
		c.EXPECT().Put(mock.Anything, userID,
			StatusSnapshot{Active: true, TokenVersion: 7}, 30*time.Second).Return(nil)
		r := New(repo, c, testhelper.DiscardLogger())

		got, err := r.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, contract.AccountStatus{Active: true, TokenVersion: 7}, got)
	})

	t.Run("falls back to the repository when the cache read errors", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(true, 3, nil)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).
			Return(StatusSnapshot{}, false, errors.New("backend down"))
		c.EXPECT().Put(mock.Anything, userID, mock.Anything, mock.Anything).Return(nil)
		r := New(repo, c, testhelper.DiscardLogger())

		got, err := r.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, contract.AccountStatus{Active: true, TokenVersion: 3}, got)
	})

	t.Run("caches an inactive user as inactive", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(false, 4, nil)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).Return(StatusSnapshot{}, false, nil)
		c.EXPECT().Put(mock.Anything, userID,
			StatusSnapshot{Active: false, TokenVersion: 4}, 30*time.Second).Return(nil)
		r := New(repo, c, testhelper.DiscardLogger())

		got, err := r.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, contract.AccountStatus{Active: false, TokenVersion: 4}, got)
	})

	t.Run("reports a deleted user as inactive rather than an error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(false, 0, apperror.ErrNotFound)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).Return(StatusSnapshot{}, false, nil)
		r := New(repo, c, testhelper.DiscardLogger())

		got, err := r.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, contract.AccountStatus{Active: false, TokenVersion: 0}, got)
	})

	t.Run("still returns the result when the cache write fails", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(true, 1, nil)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).Return(StatusSnapshot{}, false, nil)
		c.EXPECT().Put(mock.Anything, userID, mock.Anything, mock.Anything).
			Return(errors.New("backend down"))
		r := New(repo, c, testhelper.DiscardLogger())

		got, err := r.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, contract.AccountStatus{Active: true, TokenVersion: 1}, got)
	})

	t.Run("works with NoCache, always reading through to the repository", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(true, 9, nil)
		r := New(repo, NoCache{}, testhelper.DiscardLogger())

		got, err := r.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, contract.AccountStatus{Active: true, TokenVersion: 9}, got)
	})

	t.Run("repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(StatusSnapshot{}, false, nil)
		r := New(repo, c, testhelper.DiscardLogger())

		dbErr := errors.New("database timeout")
		repo.EXPECT().GetStatusByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(false, 0, dbErr)

		_, err := r.CheckStatus(context.Background(), uuid.New())
		assert.ErrorIs(t, err, dbErr)
	})
}

// GetByID serves both /users/me and the admin /users/{id} route: the old
// service had two identically-bodied methods, GetProfile and AdminGetByID,
// for those two callers. They collapse into this one method here.
func TestReader_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		expected := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Phone:     "555-1234",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(expected, nil)

		result, err := r.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := r.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestReader_ListAdmin(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		r := New(repo, NoCache{}, testhelper.DiscardLogger())

		params := Params{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}}
		users := []domain.User{
			{ID: uuid.New(), Email: "a@example.com", FirstName: "A", LastName: "User"},
			{ID: uuid.New(), Email: "b@example.com", FirstName: "B", LastName: "User"},
		}
		repo.EXPECT().ListAdmin(mock.Anything, params).Return(users, 2, nil)

		result, total, err := r.ListAdmin(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, total)
	})
}
