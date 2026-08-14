package updateprofile

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
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success partial update", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

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

		phone := "555-9999"
		result, err := cmd.Execute(context.Background(), id, Params{
			FirstName: "Alicia",
			Phone:     &phone,
		})
		require.NoError(t, err)
		assert.Equal(t, &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alicia",
			LastName:  "Smith",
			Phone:     "555-9999",
			Role:      "user",
			Active:    true,
		}, result)
	})

	t.Run("updates last name", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

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

		result, err := cmd.Execute(context.Background(), id, Params{
			LastName: "Jones",
		})
		require.NoError(t, err)
		assert.Equal(t, "Jones", result.LastName)
		assert.Equal(t, "Alice", result.FirstName)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := cmd.Execute(context.Background(), uuid.New(), Params{FirstName: "X"})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repo Update error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

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

		_, err := cmd.Execute(context.Background(), id, Params{FirstName: "Alicia"})
		assert.ErrorIs(t, err, updateErr)
	})
}
