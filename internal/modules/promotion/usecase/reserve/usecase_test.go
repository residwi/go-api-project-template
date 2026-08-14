package reserve

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
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommand_Reserve(t *testing.T) {
	t.Parallel()

	t.Run("success percentage discount", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, testhelper.FakeTxRunner{})

		promoID := uuid.New()
		userID := uuid.New()
		orderID := uuid.New()
		maxDiscount := int64(500)

		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&domain.Promotion{
				ID:             promoID,
				Code:           "SAVE20",
				Type:           domain.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				MaxDiscount:    &maxDiscount,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)
		repo.EXPECT().ApplyPromotion(mock.Anything, promoID).Return(nil)
		repo.EXPECT().CreateUsage(mock.Anything, mock.AnythingOfType("*domain.CouponUsage")).Return(nil)

		discount, err := cmd.Reserve(context.Background(), "SAVE20", userID, orderID, 2000)
		require.NoError(t, err)
		assert.Equal(t, int64(400), discount)
	})

	t.Run("validation error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, testhelper.FakeTxRunner{})

		repo.EXPECT().GetByCode(mock.Anything, "INACTIVE").
			Return(&domain.Promotion{
				ID:        uuid.New(),
				Code:      "INACTIVE",
				Active:    false,
				StartsAt:  time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil)

		discount, err := cmd.Reserve(context.Background(), "INACTIVE", uuid.New(), uuid.New(), 5000)
		assert.Equal(t, int64(0), discount)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("GetByCode error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, testhelper.FakeTxRunner{})

		repo.EXPECT().GetByCode(mock.Anything, "UNKNOWN").Return(nil, apperror.ErrNotFound)

		discount, err := cmd.Reserve(context.Background(), "UNKNOWN", uuid.New(), uuid.New(), 5000)
		assert.Equal(t, int64(0), discount)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("ApplyPromotion error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, testhelper.FakeTxRunner{})

		promoID := uuid.New()
		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&domain.Promotion{
				ID:             promoID,
				Code:           "SAVE20",
				Type:           domain.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)
		repo.EXPECT().ApplyPromotion(mock.Anything, promoID).Return(apperror.ErrCouponExhausted)

		discount, err := cmd.Reserve(context.Background(), "SAVE20", uuid.New(), uuid.New(), 2000)
		assert.Equal(t, int64(400), discount)
		assert.ErrorIs(t, err, apperror.ErrCouponExhausted)
	})

	t.Run("CreateUsage error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, testhelper.FakeTxRunner{})

		promoID := uuid.New()
		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&domain.Promotion{
				ID:             promoID,
				Code:           "SAVE20",
				Type:           domain.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)
		repo.EXPECT().ApplyPromotion(mock.Anything, promoID).Return(nil)
		repo.EXPECT().
			CreateUsage(mock.Anything, mock.AnythingOfType("*domain.CouponUsage")).
			Return(apperror.ErrConflict)

		discount, err := cmd.Reserve(context.Background(), "SAVE20", uuid.New(), uuid.New(), 2000)
		assert.Equal(t, int64(400), discount)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestCommand_Release(t *testing.T) {
	t.Parallel()

	t.Run("success releases coupon", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, testhelper.FakeTxRunner{})

		orderID := uuid.New()
		couponID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).
			Return(&domain.CouponUsage{CouponID: couponID, Discount: 400}, nil)
		repo.EXPECT().ReleasePromotion(mock.Anything, couponID).Return(nil)

		err := cmd.Release(context.Background(), orderID)
		require.NoError(t, err)
	})

	t.Run("no usage is a no-op", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, testhelper.FakeTxRunner{})

		orderID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		err := cmd.Release(context.Background(), orderID)
		require.NoError(t, err)
	})

	t.Run("DeleteUsageByOrderID error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, testhelper.FakeTxRunner{})

		orderID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).Return(nil, assert.AnError)

		err := cmd.Release(context.Background(), orderID)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("ReleasePromotion error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		cmd := New(repo, testhelper.FakeTxRunner{})

		orderID := uuid.New()
		couponID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).
			Return(&domain.CouponUsage{CouponID: couponID, Discount: 400}, nil)
		repo.EXPECT().ReleasePromotion(mock.Anything, couponID).Return(assert.AnError)

		err := cmd.Release(context.Background(), orderID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
