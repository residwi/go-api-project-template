package apply

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

	t.Run("success percentage", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		maxDiscount := int64(500)
		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&domain.Promotion{
				ID:             uuid.New(),
				Code:           "SAVE20",
				Type:           domain.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				MaxDiscount:    &maxDiscount,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := cmd.Execute(context.Background(), "SAVE20", 2000)
		require.NoError(t, err)
		assert.Equal(t, int64(400), discount)
	})

	t.Run("success percentage capped by max discount", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		maxDiscount := int64(500)
		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&domain.Promotion{
				ID:             uuid.New(),
				Code:           "SAVE20",
				Type:           domain.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				MaxDiscount:    &maxDiscount,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := cmd.Execute(context.Background(), "SAVE20", 10000)
		require.NoError(t, err)
		assert.Equal(t, int64(500), discount)
	})

	t.Run("success fixed_amount", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByCode(mock.Anything, "FLAT10").
			Return(&domain.Promotion{
				ID:             uuid.New(),
				Code:           "FLAT10",
				Type:           domain.TypeFixedAmount,
				Value:          1000,
				MinOrderAmount: 500,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := cmd.Execute(context.Background(), "FLAT10", 5000)
		require.NoError(t, err)
		assert.Equal(t, int64(1000), discount)
	})

	t.Run("inactive promo", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByCode(mock.Anything, "INACTIVE").
			Return(&domain.Promotion{
				ID:        uuid.New(),
				Code:      "INACTIVE",
				Active:    false,
				StartsAt:  time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil)

		_, err := cmd.Execute(context.Background(), "INACTIVE", 5000)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("expired", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByCode(mock.Anything, "EXPIRED").
			Return(&domain.Promotion{
				ID:        uuid.New(),
				Code:      "EXPIRED",
				Active:    true,
				StartsAt:  time.Now().Add(-2 * time.Hour),
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			}, nil)

		_, err := cmd.Execute(context.Background(), "EXPIRED", 5000)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("not started", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByCode(mock.Anything, "FUTURE").
			Return(&domain.Promotion{
				ID:        uuid.New(),
				Code:      "FUTURE",
				Active:    true,
				StartsAt:  time.Now().Add(1 * time.Hour),
				ExpiresAt: time.Now().Add(2 * time.Hour),
			}, nil)

		_, err := cmd.Execute(context.Background(), "FUTURE", 5000)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("exhausted uses", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		maxUses := 10
		repo.EXPECT().GetByCode(mock.Anything, "MAXED").
			Return(&domain.Promotion{
				ID:        uuid.New(),
				Code:      "MAXED",
				Active:    true,
				MaxUses:   &maxUses,
				UsedCount: 10,
				StartsAt:  time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil)

		_, err := cmd.Execute(context.Background(), "MAXED", 5000)
		assert.ErrorIs(t, err, apperror.ErrCouponExhausted)
	})

	t.Run("below min order", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByCode(mock.Anything, "MINORDER").
			Return(&domain.Promotion{
				ID:             uuid.New(),
				Code:           "MINORDER",
				Type:           domain.TypeFixedAmount,
				Value:          500,
				MinOrderAmount: 5000,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		_, err := cmd.Execute(context.Background(), "MINORDER", 1000)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByCode(mock.Anything, "UNKNOWN").Return(nil, apperror.ErrNotFound)

		_, err := cmd.Execute(context.Background(), "UNKNOWN", 5000)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("discount capped by subtotal", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByCode(mock.Anything, "BIG").
			Return(&domain.Promotion{
				ID:             uuid.New(),
				Code:           "BIG",
				Type:           domain.TypeFixedAmount,
				Value:          10000,
				MinOrderAmount: 100,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := cmd.Execute(context.Background(), "BIG", 500)
		require.NoError(t, err)
		assert.Equal(t, int64(500), discount)
	})

	t.Run("percentage without max discount cap", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo)

		repo.EXPECT().GetByCode(mock.Anything, "NOCAP").
			Return(&domain.Promotion{
				ID:             uuid.New(),
				Code:           "NOCAP",
				Type:           domain.TypePercentage,
				Value:          50,
				MinOrderAmount: 100,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := cmd.Execute(context.Background(), "NOCAP", 2000)
		require.NoError(t, err)
		assert.Equal(t, int64(1000), discount)
	})
}
