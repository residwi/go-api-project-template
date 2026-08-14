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

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("deletes promotion", func(t *testing.T) {
		p := seedPromotion(t)
		repo := New(testPool)

		require.NoError(t, repo.Delete(context.Background(), p.ID))

		var count int
		err := testPool.QueryRow(context.Background(), `SELECT COUNT(*) FROM promotions WHERE id = $1`, p.ID).
			Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)
		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	t.Run("Delete returns error on cancelled context", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(testPool)
		err := repo.Delete(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
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
	return p
}
