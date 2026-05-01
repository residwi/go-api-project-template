package promotion_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_promotion")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES ($1, $2, 'x', 'A', 'B')`,
		id, id.String()+"@test.com",
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id) })
	return id
}

func newPromotion(code string) *promotion.Promotion {
	maxUses := 10
	return &promotion.Promotion{
		Code:      code,
		Type:      promotion.TypePercentage,
		Value:     10,
		StartsAt:  time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(time.Hour),
		MaxUses:   &maxUses,
		Active:    true,
	}
}

func seedPromotion(t *testing.T) *promotion.Promotion {
	t.Helper()
	repo := promotion.NewPostgresRepository(testPool)
	p := newPromotion("PROMO-" + uuid.New().String()[:8])
	require.NoError(t, repo.Create(context.Background(), p))
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, p.ID) })
	return p
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates promotion", func(t *testing.T) {
		setup(t)
		repo := promotion.NewPostgresRepository(testPool)
		p := newPromotion("CREATE-" + uuid.New().String()[:8])

		err := repo.Create(context.Background(), p)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, p.ID)
		assert.Equal(t, 0, p.UsedCount)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, p.ID) })
	})

	t.Run("returns conflict error on duplicate code", func(t *testing.T) {
		setup(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)

		dup := newPromotion(p.Code)
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns promotion", func(t *testing.T) {
		setup(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, p.Code, got.Code)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := promotion.NewPostgresRepository(testPool)
		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_GetByCode(t *testing.T) {
	t.Run("returns promotion by code", func(t *testing.T) {
		setup(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)

		got, err := repo.GetByCode(context.Background(), p.Code)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := promotion.NewPostgresRepository(testPool)
		_, err := repo.GetByCode(context.Background(), "NONEXISTENT")
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates promotion fields", func(t *testing.T) {
		setup(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)

		p.Active = false
		err := repo.Update(context.Background(), p)
		require.NoError(t, err)

		got, _ := repo.GetByID(context.Background(), p.ID)
		assert.False(t, got.Active)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := promotion.NewPostgresRepository(testPool)
		p := newPromotion("NOPE-" + uuid.New().String()[:8])
		p.ID = uuid.New()
		err := repo.Update(context.Background(), p)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("returns conflict on duplicate code", func(t *testing.T) {
		setup(t)
		p1 := seedPromotion(t)
		p2 := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)

		p2.Code = p1.Code
		err := repo.Update(context.Background(), p2)
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("deletes promotion", func(t *testing.T) {
		setup(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)

		require.NoError(t, repo.Delete(context.Background(), p.ID))

		_, err := repo.GetByID(context.Background(), p.ID)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := promotion.NewPostgresRepository(testPool)
		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns paginated list", func(t *testing.T) {
		setup(t)
		seedPromotion(t)
		seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)

		items, total, err := repo.ListAdmin(context.Background(), promotion.ListParams{Page: 1, PageSize: 10})
		require.NoError(t, err)
		assert.Equal(t, 2, total)
		assert.Len(t, items, 2)
	})
}

func TestPostgresRepository_ApplyPromotion(t *testing.T) {
	t.Run("applies discount and increments uses", func(t *testing.T) {
		setup(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)

		require.NoError(t, repo.ApplyPromotion(context.Background(), p.ID))

		got, _ := repo.GetByID(context.Background(), p.ID)
		assert.Equal(t, 1, got.UsedCount)
	})

	t.Run("returns error when max uses exceeded", func(t *testing.T) {
		setup(t)
		repo := promotion.NewPostgresRepository(testPool)
		maxUses := 1
		p := &promotion.Promotion{
			Code:      "MAXED-" + uuid.New().String()[:8],
			Type:      promotion.TypePercentage,
			Value:     10,
			StartsAt:  time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(time.Hour),
			MaxUses:   &maxUses,
			Active:    true,
		}
		require.NoError(t, repo.Create(context.Background(), p))
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, p.ID) })

		// First apply succeeds
		require.NoError(t, repo.ApplyPromotion(context.Background(), p.ID))
		// Second apply exceeds max_uses
		err := repo.ApplyPromotion(context.Background(), p.ID)
		assert.ErrorIs(t, err, core.ErrCouponExhausted)
	})
}

func TestPostgresRepository_ReleasePromotion(t *testing.T) {
	t.Run("decrements uses count", func(t *testing.T) {
		setup(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)
		ctx := context.Background()

		require.NoError(t, repo.ApplyPromotion(ctx, p.ID))
		require.NoError(t, repo.ReleasePromotion(ctx, p.ID))

		got, _ := repo.GetByID(ctx, p.ID)
		assert.Equal(t, 0, got.UsedCount)
	})
}

func TestPostgresRepository_CreateUsage(t *testing.T) {
	t.Run("records coupon usage for order", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)
		ctx := context.Background()

		// Need a valid order — insert directly
		orderID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
			 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD')`,
			orderID, userID,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, orderID) })

		usage := &promotion.CouponUsage{
			CouponID: p.ID,
			UserID:   userID,
			OrderID:  orderID,
			Discount: 100,
		}
		err = repo.CreateUsage(ctx, usage)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, usage.ID)
	})

	t.Run("returns conflict on duplicate usage for same order", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)
		ctx := context.Background()

		orderID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
			 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD')`,
			orderID, userID,
		)
		require.NoError(t, err)
		t.Cleanup(func() {
			testPool.Exec(ctx, `DELETE FROM coupon_usages WHERE order_id = $1`, orderID)
			testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, orderID)
		})

		usage := &promotion.CouponUsage{
			CouponID: p.ID, UserID: userID, OrderID: orderID, Discount: 100,
		}
		require.NoError(t, repo.CreateUsage(ctx, usage))

		dup := &promotion.CouponUsage{
			CouponID: p.ID, UserID: userID, OrderID: orderID, Discount: 50,
		}
		err = repo.CreateUsage(ctx, dup)
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := promotion.NewPostgresRepository(testPool)

	t.Run("Create", func(t *testing.T) {
		setup(t)
		p := newPromotion("CANCEL-" + uuid.New().String()[:8])
		err := repo.Create(cancelledCtx, p)
		assert.Error(t, err)
	})

	t.Run("GetByID", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetByCode", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByCode(cancelledCtx, "NONEXISTENT")
		assert.Error(t, err)
	})

	t.Run("Update", func(t *testing.T) {
		setup(t)
		p := newPromotion("CANCEL-UPD-" + uuid.New().String()[:8])
		p.ID = uuid.New()
		err := repo.Update(cancelledCtx, p)
		assert.Error(t, err)
	})

	t.Run("Delete", func(t *testing.T) {
		setup(t)
		err := repo.Delete(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("ListAdmin", func(t *testing.T) {
		setup(t)
		_, _, err := repo.ListAdmin(cancelledCtx, promotion.ListParams{Page: 1, PageSize: 10})
		assert.Error(t, err)
	})

	t.Run("ApplyPromotion", func(t *testing.T) {
		setup(t)
		err := repo.ApplyPromotion(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("ReleasePromotion", func(t *testing.T) {
		setup(t)
		err := repo.ReleasePromotion(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("CreateUsage", func(t *testing.T) {
		setup(t)
		usage := &promotion.CouponUsage{CouponID: uuid.New(), UserID: uuid.New(), OrderID: uuid.New(), Discount: 100}
		err := repo.CreateUsage(cancelledCtx, usage)
		assert.Error(t, err)
	})

	t.Run("DeleteUsageByOrderID", func(t *testing.T) {
		setup(t)
		_, err := repo.DeleteUsageByOrderID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

func TestSearchString(t *testing.T) {
	t.Run("returns false when substring not found", func(t *testing.T) {
		setup(t)
		repo := promotion.NewPostgresRepository(testPool)
		p := newPromotion("SEARCH-" + uuid.New().String()[:8])
		require.NoError(t, repo.Create(context.Background(), p))
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, p.ID) })

		// Update with same code — no unique violation, exercises isUniqueViolation returning false
		p.Active = false
		err := repo.Update(context.Background(), p)
		require.NoError(t, err)
	})
}

func TestPostgresRepository_DeleteUsageByOrderID(t *testing.T) {
	t.Run("removes usage record by order id", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		p := seedPromotion(t)
		repo := promotion.NewPostgresRepository(testPool)
		ctx := context.Background()

		orderID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
			 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD')`,
			orderID, userID,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, orderID) })

		usage := &promotion.CouponUsage{
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
		setup(t)
		repo := promotion.NewPostgresRepository(testPool)
		result, err := repo.DeleteUsageByOrderID(context.Background(), uuid.New())
		require.ErrorIs(t, err, core.ErrNotFound)
		assert.Nil(t, result)
	})
}
