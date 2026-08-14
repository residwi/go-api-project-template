package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_promotion")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetByCode(t *testing.T) {
	t.Run("returns promotion by code", func(t *testing.T) {
		p := seedPromotion(t)
		repo := New(testPool)

		got, err := repo.GetByCode(context.Background(), p.Code)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)
		_, err := repo.GetByCode(context.Background(), "NONEXISTENT-"+uuid.New().String()[:8])
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_ApplyPromotion(t *testing.T) {
	t.Run("applies discount and increments uses", func(t *testing.T) {
		p := seedPromotion(t)
		repo := New(testPool)

		require.NoError(t, repo.ApplyPromotion(context.Background(), p.ID))

		var usedCount int
		err := testPool.QueryRow(context.Background(), `SELECT used_count FROM promotions WHERE id = $1`, p.ID).
			Scan(&usedCount)
		require.NoError(t, err)
		assert.Equal(t, 1, usedCount)
	})

	t.Run("returns error when max uses exceeded", func(t *testing.T) {
		repo := New(testPool)
		maxUses := 1
		p := &domain.Promotion{
			Code:      "MAXED-" + uuid.New().String()[:8],
			Type:      domain.TypePercentage,
			Value:     10,
			StartsAt:  time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(time.Hour),
			MaxUses:   &maxUses,
			Active:    true,
		}
		require.NoError(t, insertPromotion(p))
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, p.ID) })

		require.NoError(t, repo.ApplyPromotion(context.Background(), p.ID))
		err := repo.ApplyPromotion(context.Background(), p.ID)
		assert.ErrorIs(t, err, apperror.ErrCouponExhausted)
	})
}

func TestPostgresRepository_ReleasePromotion(t *testing.T) {
	t.Run("decrements uses count", func(t *testing.T) {
		p := seedPromotion(t)
		repo := New(testPool)
		ctx := context.Background()

		require.NoError(t, repo.ApplyPromotion(ctx, p.ID))
		require.NoError(t, repo.ReleasePromotion(ctx, p.ID))

		var usedCount int
		err := testPool.QueryRow(ctx, `SELECT used_count FROM promotions WHERE id = $1`, p.ID).Scan(&usedCount)
		require.NoError(t, err)
		assert.Equal(t, 0, usedCount)
	})
}

func TestPostgresRepository_CreateUsage(t *testing.T) {
	t.Run("records coupon usage for order", func(t *testing.T) {
		userID := testhelper.SeedUser(t, testPool)
		p := seedPromotion(t)
		repo := New(testPool)
		ctx := context.Background()

		orderID := seedOrder(t, userID)

		usage := &domain.CouponUsage{
			CouponID: p.ID,
			UserID:   userID,
			OrderID:  orderID,
			Discount: 100,
		}
		err := repo.CreateUsage(ctx, usage)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, usage.ID)
	})

	t.Run("returns conflict on duplicate usage for same order", func(t *testing.T) {
		userID := testhelper.SeedUser(t, testPool)
		p := seedPromotion(t)
		repo := New(testPool)
		ctx := context.Background()

		orderID := seedOrder(t, userID)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM coupon_usages WHERE order_id = $1`, orderID) })

		usage := &domain.CouponUsage{
			CouponID: p.ID, UserID: userID, OrderID: orderID, Discount: 100,
		}
		require.NoError(t, repo.CreateUsage(ctx, usage))

		dup := &domain.CouponUsage{
			CouponID: p.ID, UserID: userID, OrderID: orderID, Discount: 50,
		}
		err := repo.CreateUsage(ctx, dup)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_DeleteUsageByOrderID(t *testing.T) {
	t.Run("removes usage record by order id", func(t *testing.T) {
		userID := testhelper.SeedUser(t, testPool)
		p := seedPromotion(t)
		repo := New(testPool)
		ctx := context.Background()

		orderID := seedOrder(t, userID)

		usage := &domain.CouponUsage{
			CouponID: p.ID,
			UserID:   userID,
			OrderID:  orderID,
			Discount: 100,
		}
		require.NoError(t, repo.CreateUsage(ctx, usage))

		deleted, err := repo.DeleteUsageByOrderID(ctx, orderID)
		require.NoError(t, err)
		require.NotNil(t, deleted)
		assert.Equal(t, p.ID, deleted.CouponID)
	})

	t.Run("returns ErrNotFound when no usage exists", func(t *testing.T) {
		repo := New(testPool)
		result, err := repo.DeleteUsageByOrderID(context.Background(), uuid.New())
		require.ErrorIs(t, err, apperror.ErrNotFound)
		assert.Nil(t, result)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("GetByCode", func(t *testing.T) {
		_, err := repo.GetByCode(cancelledCtx, "NONEXISTENT")
		assert.Error(t, err)
	})

	t.Run("ApplyPromotion", func(t *testing.T) {
		err := repo.ApplyPromotion(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("ReleasePromotion", func(t *testing.T) {
		err := repo.ReleasePromotion(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("CreateUsage", func(t *testing.T) {
		usage := &domain.CouponUsage{CouponID: uuid.New(), UserID: uuid.New(), OrderID: uuid.New(), Discount: 100}
		err := repo.CreateUsage(cancelledCtx, usage)
		assert.Error(t, err)
	})

	t.Run("DeleteUsageByOrderID", func(t *testing.T) {
		_, err := repo.DeleteUsageByOrderID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

func seedOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	ctx := context.Background()
	orderID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD')`,
		orderID, userID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, orderID) })
	return orderID
}

func insertPromotion(p *domain.Promotion) error {
	return testPool.QueryRow(
		context.Background(),
		`INSERT INTO promotions (code, type, value, min_order_amount, max_discount, max_uses, starts_at, expires_at, active)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, used_count, created_at, updated_at`,
		p.Code,
		p.Type,
		p.Value,
		p.MinOrderAmount,
		p.MaxDiscount,
		p.MaxUses,
		p.StartsAt,
		p.ExpiresAt,
		p.Active,
	).Scan(&p.ID, &p.UsedCount, &p.CreatedAt, &p.UpdatedAt)
}

func seedPromotion(t *testing.T) *domain.Promotion {
	t.Helper()
	maxUses := 10
	p := &domain.Promotion{
		Code:      "PROMO-" + uuid.New().String()[:8],
		Type:      domain.TypePercentage,
		Value:     10,
		StartsAt:  time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(time.Hour),
		MaxUses:   &maxUses,
		Active:    true,
	}
	require.NoError(t, insertPromotion(p))
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, p.ID) })
	return p
}
