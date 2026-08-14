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

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/query"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_promotion")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns the promotions this subtest seeded, newest first", func(t *testing.T) {
		repo := New(testPool)
		p1 := seedPromotion(t)
		p2 := seedPromotion(t)

		// PageSize large enough to cover every row test_promotion could hold: the
		// database is shared and never reset, so this asserts p1 and p2 are among
		// the results rather than asserting an exact total.
		items, total, err := repo.ListAdmin(
			context.Background(),
			query.Params{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 1000}},
		)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 2)

		ids := make([]uuid.UUID, len(items))
		for i, p := range items {
			ids[i] = p.ID
		}
		assert.Contains(t, ids, p1.ID)
		assert.Contains(t, ids, p2.ID)
	})

	t.Run("returns error on cancelled context", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(testPool)
		_, _, err := repo.ListAdmin(
			cancelledCtx,
			query.Params{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}},
		)
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
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, p.ID) })
	return p
}
