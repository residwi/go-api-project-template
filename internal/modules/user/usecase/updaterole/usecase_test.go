package updaterole

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
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(nil)
		invalidate.EXPECT().Invalidate(mock.Anything, targetID).Return(nil)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "admin",
		})
		require.NoError(t, err)
	})

	t.Run("self-demotion blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, NewMockStatusInvalidator(t), testhelper.DiscardLogger())

		sameID := uuid.New()

		err := cmd.Execute(context.Background(), Params{
			RequesterID: sameID,
			TargetID:    sameID,
			Role:        "user",
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
			Role:        "user",
		})
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
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
			Role:        "user",
		})
		assert.ErrorIs(t, err, countErr)
	})

	t.Run("multiple admins allows demotion", func(t *testing.T) {
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
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(nil)
		invalidate.EXPECT().Invalidate(mock.Anything, targetID).Return(nil)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "user",
		})
		require.NoError(t, err)
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
			Role:        "admin",
		})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("Update error propagates", func(t *testing.T) {
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

		updateErr := errors.New("database write failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(updateErr)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "admin",
		})
		assert.ErrorIs(t, err, updateErr)
	})

	t.Run("IncrementTokenVersion error propagates", func(t *testing.T) {
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
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)

		incrErr := errors.New("token bump failed")
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(incrErr)

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "admin",
		})
		assert.ErrorIs(t, err, incrErr)
	})

	t.Run("still succeeds when cache invalidation fails", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		invalidate := NewMockStatusInvalidator(t)
		cmd := New(repo, invalidate, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).Return(&domain.User{ID: targetID, Role: "user"}, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(nil)
		invalidate.EXPECT().Invalidate(mock.Anything, targetID).Return(errors.New("cache down"))

		err := cmd.Execute(context.Background(), Params{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "admin",
		})
		require.NoError(t, err)
	})
}
