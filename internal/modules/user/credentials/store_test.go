package credentials

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user/contract"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
)

func TestStore_GetByEmail(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		repo.EXPECT().GetByEmail(mock.Anything, "alice@example.com").
			Return(&domain.User{
				ID:           id,
				Email:        "alice@example.com",
				PasswordHash: "hash123",
				FirstName:    "Alice",
				LastName:     "Smith",
				Role:         "user",
				Active:       true,
				TokenVersion: 1,
			}, nil)

		creds, err := s.GetByEmail(context.Background(), "alice@example.com")
		require.NoError(t, err)
		assert.Equal(t, contract.Credentials{
			ID:           id,
			Email:        "alice@example.com",
			PasswordHash: "hash123",
			FirstName:    "Alice",
			LastName:     "Smith",
			Role:         "user",
			Active:       true,
			TokenVersion: 1,
		}, creds)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		repo.EXPECT().GetByEmail(mock.Anything, "nobody@example.com").
			Return(nil, apperror.ErrNotFound)

		_, err := s.GetByEmail(context.Background(), "nobody@example.com")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestStore_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.User")).
			Run(func(_ context.Context, u *domain.User) {
				u.ID = uuid.New()
			}).
			Return(nil)

		result, err := s.Create(context.Background(), contract.NewUser{
			Email:        "bob@example.com",
			PasswordHash: "hashed",
			FirstName:    "Bob",
			LastName:     "Jones",
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		assert.Equal(t, contract.User{
			Email:     "bob@example.com",
			FirstName: "Bob",
			LastName:  "Jones",
			Role:      "user",
			Active:    true,
		}, result)
	})

	t.Run("conflict error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.User")).
			Return(apperror.ErrConflict)

		_, err := s.Create(context.Background(), contract.NewUser{
			Email:        "dup@example.com",
			PasswordHash: "hashed",
			FirstName:    "Dup",
			LastName:     "User",
		})
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestStore_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&domain.User{
				ID:           id,
				Email:        "alice@example.com",
				FirstName:    "Alice",
				LastName:     "Smith",
				Role:         "admin",
				Active:       true,
				TokenVersion: 3,
			}, nil)

		result, err := s.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, contract.User{
			ID:           id,
			Email:        "alice@example.com",
			FirstName:    "Alice",
			LastName:     "Smith",
			Role:         "admin",
			Active:       true,
			TokenVersion: 3,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := s.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}
