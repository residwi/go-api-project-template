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
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_order")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Apply(t *testing.T) {
	t.Run("updates order matching any of the from-statuses", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID, domain.StatusAwaitingPayment)
		repo := New(testPool)

		err := repo.Apply(context.Background(), orderID, domain.Transition{
			To:   domain.StatusExpired,
			From: []domain.Status{domain.StatusAwaitingPayment, domain.StatusCancelled},
		})
		require.NoError(t, err)

		assert.Equal(t, domain.StatusExpired, statusOf(t, orderID))
	})

	t.Run("conflict when current status is not in from", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID, domain.StatusPaid)
		repo := New(testPool)

		err := repo.Apply(context.Background(), orderID, domain.PaidTransition)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("sets the stock flags the transition carries", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID, domain.StatusPaymentProcessing)
		repo := New(testPool)

		require.NoError(t, repo.Apply(context.Background(), orderID, domain.PaidTransition))

		deducted, reversed := stockFlagsOf(t, orderID)
		assert.True(t, deducted)
		assert.False(t, reversed)
	})
}

func TestPostgresRepository_Apply_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.Apply(ctx, uuid.New(), domain.Transition{
			To:   domain.StatusPaid,
			From: []domain.Status{domain.StatusAwaitingPayment},
		})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_UpdateStatus(t *testing.T) {
	t.Run("transitions to new status", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID, domain.StatusAwaitingPayment)
		repo := New(testPool)

		err := repo.UpdateStatus(
			context.Background(),
			orderID,
			domain.StatusAwaitingPayment,
			domain.StatusPaymentProcessing,
		)
		require.NoError(t, err)

		assert.Equal(t, domain.StatusPaymentProcessing, statusOf(t, orderID))
	})

	t.Run("returns conflict when from-status does not match", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID, domain.StatusAwaitingPayment)
		repo := New(testPool)

		// paid is the wrong from-status for an awaiting_payment order.
		err := repo.UpdateStatus(context.Background(), orderID, domain.StatusPaid, domain.StatusProcessing)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_UpdateStatus_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.UpdateStatus(ctx, uuid.New(), domain.StatusAwaitingPayment, domain.StatusPaid)
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

// seedOrder inserts the minimal row Apply/UpdateStatus need: raw SQL, not
// place's repository, so this package never imports a sibling slice.
func seedOrder(t *testing.T, userID uuid.UUID, status domain.Status) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	var orderID uuid.UUID
	err := testPool.QueryRow(ctx,
		`INSERT INTO orders (user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, $3, 1000, 0, 1000, 'USD') RETURNING id`,
		userID, uuid.New().String(), status,
	).Scan(&orderID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID) })
	return orderID
}

func statusOf(t *testing.T, orderID uuid.UUID) domain.Status {
	t.Helper()
	var status domain.Status
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT status FROM orders WHERE id = $1`, orderID).Scan(&status))
	return status
}

func stockFlagsOf(t *testing.T, orderID uuid.UUID) (deducted, reversed bool) {
	t.Helper()
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT stock_deducted, stock_reversed FROM orders WHERE id = $1`, orderID).Scan(&deducted, &reversed))
	return deducted, reversed
}
