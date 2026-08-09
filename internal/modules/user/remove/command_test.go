package remove

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

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		invalidate := NewMockStatusInvalidator(t)
		cmd := New(repo, invalidate, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{
				ID:   targetID,
				Role: "user",
			}, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)
		invalidate.EXPECT().Invalidate(mock.Anything, targetID).Return(nil)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		require.NoError(t, err)
	})

	t.Run("self-deletion blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, NewMockStatusInvalidator(t), testhelper.DiscardLogger())

		sameID := uuid.New()

		err := cmd.Execute(context.Background(), Params{
			RequesterID: sameID,
			TargetID:    sameID,
		})
		assert.ErrorIs(t, err, apperror.ErrForbidden)
	})

	t.Run("last admin blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, NewMockStatusInvalidator(t), testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(1, nil)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, NewMockStatusInvalidator(t), testhelper.DiscardLogger())

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: uuid.New(),
			TargetID:    uuid.New(),
		})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("CountAdmins error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, NewMockStatusInvalidator(t), testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{
				ID:   targetID,
				Role: "admin",
			}, nil)

		countErr := errors.New("count query failed")
		repo.EXPECT().CountAdmins(mock.Anything).Return(0, countErr)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		assert.ErrorIs(t, err, countErr)
	})

	t.Run("multiple admins allows delete", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		invalidate := NewMockStatusInvalidator(t)
		cmd := New(repo, invalidate, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(3, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)
		invalidate.EXPECT().Invalidate(mock.Anything, targetID).Return(nil)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		require.NoError(t, err)
	})

	t.Run("Delete repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, NewMockStatusInvalidator(t), testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{
				ID:   targetID,
				Role: "user",
			}, nil)

		deleteErr := errors.New("database delete failed")
		repo.EXPECT().Delete(mock.Anything, targetID).Return(deleteErr)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		assert.ErrorIs(t, err, deleteErr)
	})

	t.Run("still succeeds when cache invalidation fails", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		invalidate := NewMockStatusInvalidator(t)
		cmd := New(repo, invalidate, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).Return(&domain.User{ID: targetID, Role: "user"}, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)
		invalidate.EXPECT().Invalidate(mock.Anything, targetID).Return(errors.New("cache down"))

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		require.NoError(t, err)
	})
}
