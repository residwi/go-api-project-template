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

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns promotion", func(t *testing.T) {
		p := seedPromotion(t)
		repo := New(testPool)

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, p.Code, got.Code)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)
		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates promotion fields", func(t *testing.T) {
		p := seedPromotion(t)
		repo := New(testPool)

		p.Active = false
		err := repo.Update(context.Background(), p)
		require.NoError(t, err)

		got, _ := repo.GetByID(context.Background(), p.ID)
		assert.False(t, got.Active)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)
		p := newPromotion("NOPE-" + uuid.New().String()[:8])
		p.ID = uuid.New()
		err := repo.Update(context.Background(), p)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns conflict on duplicate code", func(t *testing.T) {
		p1 := seedPromotion(t)
		p2 := seedPromotion(t)
		repo := New(testPool)

		p2.Code = p1.Code
		err := repo.Update(context.Background(), p2)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("keeping its own code is not a conflict", func(t *testing.T) {
		p := seedPromotion(t)
		repo := New(testPool)

		// Same code, so no unique violation: covers isUniqueViolation returning false.
		p.Active = false
		err := repo.Update(context.Background(), p)
		require.NoError(t, err)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(testPool)
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("GetByID", func(t *testing.T) {
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("Update", func(t *testing.T) {
		p := newPromotion("CANCEL-UPD-" + uuid.New().String()[:8])
		p.ID = uuid.New()
		err := repo.Update(cancelledCtx, p)
		assert.Error(t, err)
	})
}

func newPromotion(code string) *domain.Promotion {
	maxUses := 10
	return &domain.Promotion{
		Code:      code,
		Type:      domain.TypePercentage,
		Value:     10,
		StartsAt:  time.Now().Add(-time.Hour),
		ExpiresAt: time.Now().Add(time.Hour),
		MaxUses:   &maxUses,
		Active:    true,
	}
}

func seedPromotion(t *testing.T) *domain.Promotion {
	t.Helper()
	p := newPromotion("PROMO-" + uuid.New().String()[:8])
	err := testPool.QueryRow(
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
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, p.ID) })
	return p
}
