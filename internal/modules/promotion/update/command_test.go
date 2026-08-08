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
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success partial", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		id := uuid.New()
		existing := &domain.Promotion{
			ID:             id,
			Code:           "OLD",
			Type:           domain.TypeFixedAmount,
			Value:          500,
			MinOrderAmount: 1000,
			Active:         true,
			StartsAt:       time.Now().Add(-time.Hour),
			ExpiresAt:      time.Now().Add(time.Hour),
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.Promotion")).Return(nil)

		newValue := int64(750)
		result, err := cmd.Execute(context.Background(), id, Params{
			Code:  "UPDATED",
			Value: &newValue,
		})
		require.NoError(t, err)
		assert.Equal(t, &domain.Promotion{
			ID:             id,
			Code:           "UPDATED",
			Type:           domain.TypeFixedAmount,
			Value:          750,
			MinOrderAmount: 1000,
			Active:         true,
			StartsAt:       existing.StartsAt,
			ExpiresAt:      existing.ExpiresAt,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := cmd.Execute(context.Background(), uuid.New(), Params{Code: "X"})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		id := uuid.New()
		existing := &domain.Promotion{
			ID:        id,
			Code:      "OLD",
			Type:      domain.TypeFixedAmount,
			Value:     500,
			Active:    true,
			StartsAt:  time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(time.Hour),
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.Promotion")).Return(apperror.ErrConflict)

		_, err := cmd.Execute(context.Background(), id, Params{Code: "DUP"})
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("all fields updated", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		id := uuid.New()
		existing := &domain.Promotion{
			ID:             id,
			Code:           "OLD",
			Type:           domain.TypeFixedAmount,
			Value:          500,
			MinOrderAmount: 1000,
			Active:         true,
			StartsAt:       time.Now().Add(-time.Hour),
			ExpiresAt:      time.Now().Add(time.Hour),
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.Promotion")).Return(nil)

		newValue := int64(75)
		newMinOrder := int64(2000)
		newMaxDiscount := int64(300)
		newMaxUses := 50
		newStartsAt := time.Now().Add(-2 * time.Hour)
		newExpiresAt := time.Now().Add(2 * time.Hour)
		newActive := false

		result, err := cmd.Execute(context.Background(), id, Params{
			Code:           "NEWCODE",
			Type:           domain.TypePercentage,
			Value:          &newValue,
			MinOrderAmount: &newMinOrder,
			MaxDiscount:    &newMaxDiscount,
			MaxUses:        &newMaxUses,
			StartsAt:       &newStartsAt,
			ExpiresAt:      &newExpiresAt,
			Active:         &newActive,
		})
		require.NoError(t, err)
		assert.Equal(t, &domain.Promotion{
			ID:             id,
			Code:           "NEWCODE",
			Type:           domain.TypePercentage,
			Value:          75,
			MinOrderAmount: 2000,
			MaxDiscount:    &newMaxDiscount,
			MaxUses:        &newMaxUses,
			StartsAt:       newStartsAt,
			ExpiresAt:      newExpiresAt,
			Active:         false,
		}, result)
	})
}
