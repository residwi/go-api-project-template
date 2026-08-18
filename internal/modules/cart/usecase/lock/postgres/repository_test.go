package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_cart")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

// This is order/place's only proof that the row lock backing checkout
// serialization exists: it is what SELECT ... FOR UPDATE returns for an
// absent vs. an existing cart, the two states order/place's CartLocker.Lock
// distinguishes.
func TestPostgresRepository_GetCartForLock(t *testing.T) {
	t.Run("returns not found when cart does not exist", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		_, err := repo.GetCartForLock(context.Background(), userID)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("returns cart id when cart exists", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)
		ctx := context.Background()

		cartID := seedCart(t, userID)
		lockedID, err := repo.GetCartForLock(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, cartID, lockedID)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("GetCartForLock", func(t *testing.T) {
		_, err := repo.GetCartForLock(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

func seedCart(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	var cartID uuid.UUID
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO carts (user_id) VALUES ($1) RETURNING id`, userID).Scan(&cartID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM carts WHERE id = $1`, cartID) })
	return cartID
}
