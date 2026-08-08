package update

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success partial update", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		id := uuid.New()
		existing := &domain.Category{
			ID:     id,
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(c *domain.Category) bool {
			return c.Name == "Gadgets" && c.Slug == "gadgets"
		})).Return(nil)

		newName := "Gadgets"
		result, err := cmd.Execute(context.Background(), id, Params{
			Name: &newName,
		})

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &domain.Category{
			Name:   "Gadgets",
			Slug:   "gadgets",
			Active: true,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, apperror.ErrNotFound)

		newName := "Gadgets"
		result, err := cmd.Execute(context.Background(), id, Params{
			Name: &newName,
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		id := uuid.New()
		existing := &domain.Category{
			ID:     id,
			Name:   "Electronics",
			Slug:   "electronics",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		newName := "Gadgets"
		result, err := cmd.Execute(context.Background(), id, Params{
			Name: &newName,
		})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("updates all optional fields", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		id := uuid.New()
		existing := &domain.Category{
			ID:     id,
			Name:   "Old",
			Slug:   "old",
			Active: true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)

		newName := "New"
		newDesc := "A description"
		newSort := 10
		newActive := false
		result, err := cmd.Execute(context.Background(), id, Params{
			Name:        &newName,
			Description: &newDesc,
			SortOrder:   &newSort,
			Active:      &newActive,
		})

		require.NoError(t, err)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &domain.Category{
			Name:        "New",
			Slug:        "new",
			Description: &newDesc,
			SortOrder:   10,
			Active:      false,
		}, result)
	})
}

func TestCommand_ValidateParent(t *testing.T) {
	t.Parallel()

	t.Run("rejects a move that the repository reports as circular", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		selfID, parentID := uuid.New(), uuid.New()
		// Update loads the category being moved via GetByID(id) before it ever
		// validates the new parent; validateParent itself never calls GetByID.
		repo.EXPECT().GetByID(mock.Anything, selfID).
			Return(&domain.Category{ID: selfID, Name: "A"}, nil)
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, selfID, mock.Anything).
			Return(2, true, nil)
		cmd := New(repo)

		_, err := cmd.Execute(context.Background(), selfID, Params{ParentID: &parentID})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.ErrorContains(t, err, "circular parent reference")
	})

	t.Run("rejects a category set as its own parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		selfID := uuid.New()
		// Update calls GetByID(id) first, unconditionally, whatever the check below does.
		repo.EXPECT().GetByID(mock.Anything, selfID).
			Return(&domain.Category{ID: selfID, Name: "A"}, nil)
		cmd := New(repo)

		_, err := cmd.Execute(context.Background(), selfID, Params{ParentID: &selfID})

		require.ErrorIs(t, err, apperror.ErrBadRequest)
		assert.ErrorContains(t, err, "cannot be its own parent")
	})

	t.Run("moves a category to a valid new parent", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		selfID, parentID := uuid.New(), uuid.New()
		existing := &domain.Category{ID: selfID, Name: "Child", Slug: "child"}
		repo.EXPECT().GetByID(mock.Anything, selfID).Return(existing, nil)
		repo.EXPECT().AncestorDepthAndCycle(mock.Anything, parentID, selfID, mock.Anything).
			Return(1, false, nil)
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(c *domain.Category) bool {
			return c.ID == selfID && c.ParentID != nil && *c.ParentID == parentID
		})).Return(nil)
		cmd := New(repo)

		result, err := cmd.Execute(context.Background(), selfID, Params{ParentID: &parentID})

		require.NoError(t, err)
		assert.Equal(t, &parentID, result.ParentID)
	})
}
