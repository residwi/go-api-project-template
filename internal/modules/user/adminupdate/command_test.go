package adminupdate

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success updates active status", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		invalidate := NewMockStatusInvalidator(t)
		cmd := New(repo, invalidate, testhelper.DiscardLogger())

		id := uuid.New()
		existing := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
		invalidate.EXPECT().Invalidate(mock.Anything, id).Return(nil)

		active := false
		result, err := cmd.Execute(context.Background(), id, Params{
			Active: &active,
		})
		require.NoError(t, err)
		assert.Equal(t, &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    false,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, NewMockStatusInvalidator(t), testhelper.DiscardLogger())

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := cmd.Execute(context.Background(), uuid.New(), Params{})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repo Update error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, NewMockStatusInvalidator(t), testhelper.DiscardLogger())

		id := uuid.New()
		existing := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)

		updateErr := errors.New("database write failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(updateErr)

		_, err := cmd.Execute(context.Background(), id, Params{FirstName: "Bob"})
		assert.ErrorIs(t, err, updateErr)
	})

	t.Run("partial update with all fields", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		invalidate := NewMockStatusInvalidator(t)
		cmd := New(repo, invalidate, testhelper.DiscardLogger())

		id := uuid.New()
		existing := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Phone:     "555-0000",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
		invalidate.EXPECT().Invalidate(mock.Anything, id).Return(nil)

		phone := "555-9999"
		active := false
		result, err := cmd.Execute(context.Background(), id, Params{
			FirstName: "Bob",
			LastName:  "Jones",
			Phone:     &phone,
			Active:    &active,
		})
		require.NoError(t, err)
		assert.Equal(t, &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Bob",
			LastName:  "Jones",
			Phone:     "555-9999",
			Role:      "user",
			Active:    false,
		}, result)
	})

	t.Run("still succeeds when cache invalidation fails", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		invalidate := NewMockStatusInvalidator(t)
		cmd := New(repo, invalidate, testhelper.DiscardLogger())

		id := uuid.New()
		existing := &domain.User{
			ID:     id,
			Email:  "alice@example.com",
			Role:   "user",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
		invalidate.EXPECT().Invalidate(mock.Anything, id).Return(errors.New("cache down"))

		active := false
		result, err := cmd.Execute(context.Background(), id, Params{Active: &active})
		require.NoError(t, err)
		assert.False(t, result.Active)
	})
}
