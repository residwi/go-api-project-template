package create

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

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Promotion")).
			Run(func(_ context.Context, p *domain.Promotion) {
				p.ID = uuid.New()
				p.CreatedAt = time.Now()
				p.UpdatedAt = time.Now()
			}).
			Return(nil)

		startsAt := time.Now()
		expiresAt := time.Now().Add(24 * time.Hour)
		result, err := cmd.Execute(context.Background(), Params{
			Code:           "NEW10",
			Type:           domain.TypePercentage,
			Value:          10,
			MinOrderAmount: 1000,
			StartsAt:       startsAt,
			ExpiresAt:      expiresAt,
			Active:         true,
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		result.CreatedAt = time.Time{}
		result.UpdatedAt = time.Time{}
		assert.Equal(t, &domain.Promotion{
			Code:           "NEW10",
			Type:           domain.TypePercentage,
			Value:          10,
			MinOrderAmount: 1000,
			StartsAt:       startsAt,
			ExpiresAt:      expiresAt,
			Active:         true,
		}, result)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Promotion")).
			Return(apperror.ErrConflict)

		_, err := cmd.Execute(context.Background(), Params{
			Code:      "DUP",
			Type:      domain.TypePercentage,
			Value:     10,
			StartsAt:  time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
			Active:    true,
		})
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}
