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

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates promotion", func(t *testing.T) {
		repo := New(testPool)
		p := newPromotion("CREATE-" + uuid.New().String()[:8])

		err := repo.Create(context.Background(), p)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, p.ID)
		assert.Equal(t, 0, p.UsedCount)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, p.ID) })
	})

	t.Run("returns conflict error on duplicate code", func(t *testing.T) {
		repo := New(testPool)
		existing := newPromotion("CREATE-DUP-" + uuid.New().String()[:8])
		require.NoError(t, repo.Create(context.Background(), existing))
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM promotions WHERE id = $1`, existing.ID)
		})

		dup := newPromotion(existing.Code)
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	t.Run("Create returns error on cancelled context", func(t *testing.T) {
		cancelledCtx, cancel := context.WithCancel(context.Background())
		cancel()

		repo := New(testPool)
		p := newPromotion("CANCEL-" + uuid.New().String()[:8])
		err := repo.Create(cancelledCtx, p)
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
