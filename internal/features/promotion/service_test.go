package promotion_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	mocks "github.com/residwi/go-api-project-template/mocks/promotion"
)

type noopDBTX struct{}

func (noopDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (noopDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil } //nolint:nilnil // test stub
func (noopDBTX) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }

func TestService_Validate(t *testing.T) {
	t.Run("success percentage", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		maxDiscount := int64(500)
		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&promotion.Promotion{
				ID:             uuid.New(),
				Code:           "SAVE20",
				Type:           promotion.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				MaxDiscount:    &maxDiscount,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := svc.Validate(context.Background(), "SAVE20", 2000)
		require.NoError(t, err)
		assert.Equal(t, int64(400), discount) // 20% of 2000 = 400, under max 500
	})

	t.Run("success percentage capped by max discount", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		maxDiscount := int64(500)
		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&promotion.Promotion{
				ID:             uuid.New(),
				Code:           "SAVE20",
				Type:           promotion.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				MaxDiscount:    &maxDiscount,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := svc.Validate(context.Background(), "SAVE20", 10000)
		require.NoError(t, err)
		assert.Equal(t, int64(500), discount) // 20% of 10000 = 2000, capped at 500
	})

	t.Run("success fixed_amount", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().GetByCode(mock.Anything, "FLAT10").
			Return(&promotion.Promotion{
				ID:             uuid.New(),
				Code:           "FLAT10",
				Type:           promotion.TypeFixedAmount,
				Value:          1000,
				MinOrderAmount: 500,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := svc.Validate(context.Background(), "FLAT10", 5000)
		require.NoError(t, err)
		assert.Equal(t, int64(1000), discount)
	})

	t.Run("inactive promo", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().GetByCode(mock.Anything, "INACTIVE").
			Return(&promotion.Promotion{
				ID:        uuid.New(),
				Code:      "INACTIVE",
				Active:    false,
				StartsAt:  time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil)

		_, err := svc.Validate(context.Background(), "INACTIVE", 5000)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("expired", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().GetByCode(mock.Anything, "EXPIRED").
			Return(&promotion.Promotion{
				ID:        uuid.New(),
				Code:      "EXPIRED",
				Active:    true,
				StartsAt:  time.Now().Add(-2 * time.Hour),
				ExpiresAt: time.Now().Add(-1 * time.Hour),
			}, nil)

		_, err := svc.Validate(context.Background(), "EXPIRED", 5000)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("not started", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().GetByCode(mock.Anything, "FUTURE").
			Return(&promotion.Promotion{
				ID:        uuid.New(),
				Code:      "FUTURE",
				Active:    true,
				StartsAt:  time.Now().Add(1 * time.Hour),
				ExpiresAt: time.Now().Add(2 * time.Hour),
			}, nil)

		_, err := svc.Validate(context.Background(), "FUTURE", 5000)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("exhausted uses", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		maxUses := 10
		repo.EXPECT().GetByCode(mock.Anything, "MAXED").
			Return(&promotion.Promotion{
				ID:        uuid.New(),
				Code:      "MAXED",
				Active:    true,
				MaxUses:   &maxUses,
				UsedCount: 10,
				StartsAt:  time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil)

		_, err := svc.Validate(context.Background(), "MAXED", 5000)
		assert.ErrorIs(t, err, core.ErrCouponExhausted)
	})

	t.Run("below min order", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().GetByCode(mock.Anything, "MINORDER").
			Return(&promotion.Promotion{
				ID:             uuid.New(),
				Code:           "MINORDER",
				Type:           promotion.TypeFixedAmount,
				Value:          500,
				MinOrderAmount: 5000,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		_, err := svc.Validate(context.Background(), "MINORDER", 1000)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().GetByCode(mock.Anything, "UNKNOWN").Return(nil, core.ErrNotFound)

		_, err := svc.Validate(context.Background(), "UNKNOWN", 5000)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("discount capped by subtotal", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().GetByCode(mock.Anything, "BIG").
			Return(&promotion.Promotion{
				ID:             uuid.New(),
				Code:           "BIG",
				Type:           promotion.TypeFixedAmount,
				Value:          10000,
				MinOrderAmount: 100,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := svc.Validate(context.Background(), "BIG", 500)
		require.NoError(t, err)
		assert.Equal(t, int64(500), discount)
	})

	t.Run("percentage without max discount cap", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().GetByCode(mock.Anything, "NOCAP").
			Return(&promotion.Promotion{
				ID:             uuid.New(),
				Code:           "NOCAP",
				Type:           promotion.TypePercentage,
				Value:          50,
				MinOrderAmount: 100,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)

		discount, err := svc.Validate(context.Background(), "NOCAP", 2000)
		require.NoError(t, err)
		assert.Equal(t, int64(1000), discount)
	})
}

func TestService_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).
			Run(func(_ context.Context, p *promotion.Promotion) {
				p.ID = uuid.New()
				p.CreatedAt = time.Now()
				p.UpdatedAt = time.Now()
			}).
			Return(nil)

		startsAt := time.Now()
		expiresAt := time.Now().Add(24 * time.Hour)
		result, err := svc.Create(context.Background(), promotion.CreateRequest{
			Code:           "NEW10",
			Type:           promotion.TypePercentage,
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
		assert.Equal(t, &promotion.Promotion{
			Code:           "NEW10",
			Type:           promotion.TypePercentage,
			Value:          10,
			MinOrderAmount: 1000,
			StartsAt:       startsAt,
			ExpiresAt:      expiresAt,
			Active:         true,
		}, result)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).
			Return(core.ErrConflict)

		_, err := svc.Create(context.Background(), promotion.CreateRequest{
			Code:      "DUP",
			Type:      promotion.TypePercentage,
			Value:     10,
			StartsAt:  time.Now(),
			ExpiresAt: time.Now().Add(time.Hour),
			Active:    true,
		})
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestService_List(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		params := promotion.ListParams{Page: 1, PageSize: 10}
		promos := []promotion.Promotion{
			{ID: uuid.New(), Code: "A"},
			{ID: uuid.New(), Code: "B"},
		}
		repo.EXPECT().ListAdmin(mock.Anything, params).Return(promos, 2, nil)

		result, total, err := svc.List(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, total)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		params := promotion.ListParams{Page: 1, PageSize: 10}
		repo.EXPECT().ListAdmin(mock.Anything, params).Return(nil, 0, assert.AnError)

		_, _, err := svc.List(context.Background(), params)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_Update(t *testing.T) {
	t.Run("success partial", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		id := uuid.New()
		existing := &promotion.Promotion{
			ID:             id,
			Code:           "OLD",
			Type:           promotion.TypeFixedAmount,
			Value:          500,
			MinOrderAmount: 1000,
			Active:         true,
			StartsAt:       time.Now().Add(-time.Hour),
			ExpiresAt:      time.Now().Add(time.Hour),
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).Return(nil)

		newValue := int64(750)
		result, err := svc.Update(context.Background(), id, promotion.UpdateRequest{
			Code:  "UPDATED",
			Value: &newValue,
		})
		require.NoError(t, err)
		assert.Equal(t, &promotion.Promotion{
			ID:             id,
			Code:           "UPDATED",
			Type:           promotion.TypeFixedAmount,
			Value:          750,
			MinOrderAmount: 1000,
			Active:         true,
			StartsAt:       existing.StartsAt,
			ExpiresAt:      existing.ExpiresAt,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, core.ErrNotFound)

		_, err := svc.Update(context.Background(), uuid.New(), promotion.UpdateRequest{Code: "X"})
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("update repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		id := uuid.New()
		existing := &promotion.Promotion{
			ID:        id,
			Code:      "OLD",
			Type:      promotion.TypeFixedAmount,
			Value:     500,
			Active:    true,
			StartsAt:  time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(time.Hour),
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).Return(core.ErrConflict)

		_, err := svc.Update(context.Background(), id, promotion.UpdateRequest{Code: "DUP"})
		assert.ErrorIs(t, err, core.ErrConflict)
	})

	t.Run("all fields updated", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		id := uuid.New()
		existing := &promotion.Promotion{
			ID:             id,
			Code:           "OLD",
			Type:           promotion.TypeFixedAmount,
			Value:          500,
			MinOrderAmount: 1000,
			Active:         true,
			StartsAt:       time.Now().Add(-time.Hour),
			ExpiresAt:      time.Now().Add(time.Hour),
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).Return(nil)

		newValue := int64(750)
		newMinOrder := int64(2000)
		newMaxDiscount := int64(300)
		newMaxUses := 50
		newStartsAt := time.Now().Add(-2 * time.Hour)
		newExpiresAt := time.Now().Add(2 * time.Hour)
		newActive := false

		result, err := svc.Update(context.Background(), id, promotion.UpdateRequest{
			Code:           "NEWCODE",
			Type:           promotion.TypePercentage,
			Value:          &newValue,
			MinOrderAmount: &newMinOrder,
			MaxDiscount:    &newMaxDiscount,
			MaxUses:        &newMaxUses,
			StartsAt:       &newStartsAt,
			ExpiresAt:      &newExpiresAt,
			Active:         &newActive,
		})
		require.NoError(t, err)
		assert.Equal(t, &promotion.Promotion{
			ID:             id,
			Code:           "NEWCODE",
			Type:           promotion.TypePercentage,
			Value:          750,
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
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		err := svc.Delete(context.Background(), id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(core.ErrNotFound)

		err := svc.Delete(context.Background(), id)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestService_Reserve(t *testing.T) {
	t.Run("success percentage discount", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)
		ctx := database.WithTestTx(context.Background(), noopDBTX{})

		promoID := uuid.New()
		userID := uuid.New()
		orderID := uuid.New()
		maxDiscount := int64(500)

		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&promotion.Promotion{
				ID:             promoID,
				Code:           "SAVE20",
				Type:           promotion.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				MaxDiscount:    &maxDiscount,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)
		repo.EXPECT().ApplyPromotion(mock.Anything, promoID).Return(nil)
		repo.EXPECT().CreateUsage(mock.Anything, mock.AnythingOfType("*promotion.CouponUsage")).Return(nil)

		discount, err := svc.Reserve(ctx, "SAVE20", userID, orderID, 2000)
		require.NoError(t, err)
		assert.Equal(t, int64(400), discount)
	})

	t.Run("validation error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)
		ctx := database.WithTestTx(context.Background(), noopDBTX{})

		repo.EXPECT().GetByCode(mock.Anything, "INACTIVE").
			Return(&promotion.Promotion{
				ID:        uuid.New(),
				Code:      "INACTIVE",
				Active:    false,
				StartsAt:  time.Now().Add(-time.Hour),
				ExpiresAt: time.Now().Add(time.Hour),
			}, nil)

		discount, err := svc.Reserve(ctx, "INACTIVE", uuid.New(), uuid.New(), 5000)
		assert.Equal(t, int64(0), discount)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("GetByCode error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)
		ctx := database.WithTestTx(context.Background(), noopDBTX{})

		repo.EXPECT().GetByCode(mock.Anything, "UNKNOWN").Return(nil, core.ErrNotFound)

		discount, err := svc.Reserve(ctx, "UNKNOWN", uuid.New(), uuid.New(), 5000)
		assert.Equal(t, int64(0), discount)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("ApplyPromotion error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)
		ctx := database.WithTestTx(context.Background(), noopDBTX{})

		promoID := uuid.New()
		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&promotion.Promotion{
				ID:             promoID,
				Code:           "SAVE20",
				Type:           promotion.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)
		repo.EXPECT().ApplyPromotion(mock.Anything, promoID).Return(core.ErrCouponExhausted)

		discount, err := svc.Reserve(ctx, "SAVE20", uuid.New(), uuid.New(), 2000)
		assert.Equal(t, int64(400), discount)
		assert.ErrorIs(t, err, core.ErrCouponExhausted)
	})

	t.Run("CreateUsage error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)
		ctx := database.WithTestTx(context.Background(), noopDBTX{})

		promoID := uuid.New()
		repo.EXPECT().GetByCode(mock.Anything, "SAVE20").
			Return(&promotion.Promotion{
				ID:             promoID,
				Code:           "SAVE20",
				Type:           promotion.TypePercentage,
				Value:          20,
				MinOrderAmount: 1000,
				Active:         true,
				StartsAt:       time.Now().Add(-time.Hour),
				ExpiresAt:      time.Now().Add(time.Hour),
			}, nil)
		repo.EXPECT().ApplyPromotion(mock.Anything, promoID).Return(nil)
		repo.EXPECT().CreateUsage(mock.Anything, mock.AnythingOfType("*promotion.CouponUsage")).Return(core.ErrConflict)

		discount, err := svc.Reserve(ctx, "SAVE20", uuid.New(), uuid.New(), 2000)
		assert.Equal(t, int64(400), discount)
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestService_Release(t *testing.T) {
	t.Run("success releases coupon", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)
		ctx := database.WithTestTx(context.Background(), noopDBTX{})

		orderID := uuid.New()
		couponID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).
			Return(&promotion.CouponUsage{CouponID: couponID, Discount: 400}, nil)
		repo.EXPECT().ReleasePromotion(mock.Anything, couponID).Return(nil)

		err := svc.Release(ctx, orderID)
		require.NoError(t, err)
	})

	t.Run("no usage is a no-op", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)
		ctx := database.WithTestTx(context.Background(), noopDBTX{})

		orderID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		err := svc.Release(ctx, orderID)
		require.NoError(t, err)
	})

	t.Run("DeleteUsageByOrderID error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)
		ctx := database.WithTestTx(context.Background(), noopDBTX{})

		orderID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).Return(nil, assert.AnError)

		err := svc.Release(ctx, orderID)
		assert.ErrorIs(t, err, assert.AnError)
	})

	t.Run("ReleasePromotion error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := promotion.NewService(repo, nil)
		ctx := database.WithTestTx(context.Background(), noopDBTX{})

		orderID := uuid.New()
		couponID := uuid.New()
		repo.EXPECT().DeleteUsageByOrderID(mock.Anything, orderID).
			Return(&promotion.CouponUsage{CouponID: couponID, Discount: 400}, nil)
		repo.EXPECT().ReleasePromotion(mock.Anything, couponID).Return(assert.AnError)

		err := svc.Release(ctx, orderID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
