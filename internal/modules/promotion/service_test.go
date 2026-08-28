package promotion

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
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

func TestService_Apply(t *testing.T) {
	t.Parallel()

	t.Run("success percentage", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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

		discount, err := svc.Apply(context.Background(), "SAVE20", 2000)
		require.NoError(t, err)
		assert.Equal(t, int64(400), discount)
	})

	t.Run("success percentage capped by max discount", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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

		discount, err := svc.Apply(context.Background(), "SAVE20", 10000)
		require.NoError(t, err)
		assert.Equal(t, int64(500), discount)
	})

	t.Run("success fixed_amount", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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

		discount, err := svc.Apply(context.Background(), "FLAT10", 5000)
		require.NoError(t, err)
		assert.Equal(t, int64(1000), discount)
	})

	t.Run("inactive promo", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

		repo.EXPECT().GetByCode(mock.Anything, "INACTIVE").
			Return(&domain.Promotion{
				ID:        uuid.New(),
				Code:      "INACTIVE",
				Active:    false,
				StartsAt:  time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil)

		_, err := svc.Apply(context.Background(), "INACTIVE", 5000)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("expired", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

		repo.EXPECT().GetByCode(mock.Anything, "EXPIRED").
			Return(&domain.Promotion{
				ID:        uuid.New(),
				Code:      "EXPIRED",
				Active:    true,
				StartsAt:  time.Now().Add(-2 * time.Hour),
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			}, nil)

		_, err := svc.Apply(context.Background(), "EXPIRED", 5000)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("not started", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

		repo.EXPECT().GetByCode(mock.Anything, "FUTURE").
			Return(&domain.Promotion{
				ID:        uuid.New(),
				Code:      "FUTURE",
				Active:    true,
				StartsAt:  time.Now().Add(1 * time.Hour),
				ExpiresAt: time.Now().Add(2 * time.Hour),
			}, nil)

		_, err := svc.Apply(context.Background(), "FUTURE", 5000)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("exhausted uses", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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

		_, err := svc.Apply(context.Background(), "MAXED", 5000)
		assert.ErrorIs(t, err, apperror.ErrCouponExhausted)
	})

	t.Run("below min order", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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

		_, err := svc.Apply(context.Background(), "MINORDER", 1000)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

		repo.EXPECT().GetByCode(mock.Anything, "UNKNOWN").Return(nil, errs.ErrNotFound)

		_, err := svc.Apply(context.Background(), "UNKNOWN", 5000)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("discount capped by subtotal", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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

		discount, err := svc.Apply(context.Background(), "BIG", 500)
		require.NoError(t, err)
		assert.Equal(t, int64(500), discount)
	})

	t.Run("percentage without max discount cap", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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

		discount, err := svc.Apply(context.Background(), "NOCAP", 2000)
		require.NoError(t, err)
		assert.Equal(t, int64(1000), discount)
	})
}

func TestService_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Promotion")).
			Run(func(_ context.Context, p *domain.Promotion) {
				p.ID = uuid.New()
				p.CreatedAt = time.Now()
				p.UpdatedAt = time.Now()
			}).
			Return(nil)

		startsAt := time.Now()
		expiresAt := time.Now().Add(24 * time.Hour)
		result, err := svc.Create(
			context.Background(), "NEW10", domain.TypePercentage, 10, 1000, nil, nil, startsAt, expiresAt, true,
		)
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
		var tx database.TxRunner
		svc := New(repo, tx)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Promotion")).
			Return(errs.ErrConflict)

		_, err := svc.Create(
			context.Background(), "DUP", domain.TypePercentage, 10, 0, nil, nil,
			time.Now(), time.Now().Add(time.Hour), true,
		)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestService_Update(t *testing.T) {
	t.Parallel()

	t.Run("success partial", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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
		result, err := svc.Update(context.Background(), id, "UPDATED", "", &newValue, nil, nil, nil, nil, nil, nil)
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
		var tx database.TxRunner
		svc := New(repo, tx)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		_, err := svc.Update(context.Background(), uuid.New(), "X", "", nil, nil, nil, nil, nil, nil, nil)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.Promotion")).Return(errs.ErrConflict)

		_, err := svc.Update(context.Background(), id, "DUP", "", nil, nil, nil, nil, nil, nil, nil)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})

	t.Run("all fields updated", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

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

		result, err := svc.Update(
			context.Background(), id, "NEWCODE", domain.TypePercentage,
			&newValue, &newMinOrder, &newMaxDiscount, &newMaxUses, &newStartsAt, &newExpiresAt, &newActive,
		)
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

func TestService_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := svc.Delete(context.Background(), id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(errs.ErrNotFound)

		err := svc.Delete(context.Background(), id)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestService_ListAdmin(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

		params := AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}}
		promos := []domain.Promotion{
			{ID: uuid.New(), Code: "A"},
			{ID: uuid.New(), Code: "B"},
		}
		repo.EXPECT().ListAdmin(mock.Anything, params).Return(promos, 2, nil)

		result, total, err := svc.ListAdmin(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, total)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		var tx database.TxRunner
		svc := New(repo, tx)

		params := AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}}
		repo.EXPECT().ListAdmin(mock.Anything, params).Return(nil, 0, assert.AnError)

		_, _, err := svc.ListAdmin(context.Background(), params)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_Reserve(t *testing.T) {
	t.Parallel()

	t.Run("success percentage discount", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := New(repo, testutil.FakeTxRunner{})

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

		discount, err := svc.Reserve(context.Background(), "SAVE20", userID, orderID, 2000)
		require.NoError(t, err)
		assert.Equal(t, int64(400), discount)
	})

	t.Run("validation error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := New(repo, testutil.FakeTxRunner{})

		repo.EXPECT().GetByCode(mock.Anything, "INACTIVE").
			Return(&domain.Promotion{
				ID:        uuid.New(),
				Code:      "INACTIVE",
				Active:    false,
				StartsAt:  time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil)

		discount, err := svc.Reserve(context.Background(), "INACTIVE", uuid.New(), uuid.New(), 5000)
		assert.Equal(t, int64(0), discount)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("GetByCode error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := New(repo, testutil.FakeTxRunner{})

		repo.EXPECT().GetByCode(mock.Anything, "UNKNOWN").Return(nil, errs.ErrNotFound)

		discount, err := svc.Reserve(context.Background(), "UNKNOWN", uuid.New(), uuid.New(), 5000)
		assert.Equal(t, int64(0), discount)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("ApplyPromotion error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := New(repo, testutil.FakeTxRunner{})

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

		discount, err := svc.Reserve(context.Background(), "SAVE20", uuid.New(), uuid.New(), 2000)
		assert.Equal(t, int64(400), discount)
		assert.ErrorIs(t, err, apperror.ErrCouponExhausted)
	})

	t.Run("CreateUsage error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := New(repo, testutil.FakeTxRunner{})

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
			Return(errs.ErrConflict)

		discount, err := svc.Reserve(context.Background(), "SAVE20", uuid.New(), uuid.New(), 2000)
		assert.Equal(t, int64(400), discount)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestService_Release(t *testing.T) {
	t.Parallel()

	t.Run("success releases coupon", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := New(repo, testutil.FakeTxRunner{})

		orderID := uuid.New()
		couponID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).
			Return(&domain.CouponUsage{CouponID: couponID, Discount: 400}, nil)
		repo.EXPECT().ReleasePromotion(mock.Anything, couponID).Return(nil)

		err := svc.Release(context.Background(), orderID)
		require.NoError(t, err)
	})

	t.Run("no usage is a no-op", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := New(repo, testutil.FakeTxRunner{})

		orderID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).Return(nil, errs.ErrNotFound)

		err := svc.Release(context.Background(), orderID)
		require.NoError(t, err)
	})

	t.Run("DeleteUsageByOrderID error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := New(repo, testutil.FakeTxRunner{})

		orderID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).Return(nil, assert.AnError)

		err := svc.Release(context.Background(), orderID)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("ReleasePromotion error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := New(repo, testutil.FakeTxRunner{})

		orderID := uuid.New()
		couponID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).
			Return(&domain.CouponUsage{CouponID: couponID, Discount: 400}, nil)
		repo.EXPECT().ReleasePromotion(mock.Anything, couponID).Return(assert.AnError)

		err := svc.Release(context.Background(), orderID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
