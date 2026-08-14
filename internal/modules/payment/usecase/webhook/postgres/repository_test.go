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
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_payment")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns payment", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := New(testPool)

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, p.OrderID, got.OrderID)
		assert.Equal(t, p.Amount, got.Amount)
		assert.Equal(t, p.Status, got.Status)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetByID_WithNullableFields(t *testing.T) {
	t.Run("returns payment with payment_method_id", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1 WHERE id = $2`, "pm_webhook_get_test", p.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "pm_webhook_get_test", got.PaymentMethodID)
	})
}

func TestPostgresRepository_GetByGatewayTxnID(t *testing.T) {
	t.Run("returns payment by txn id", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		txnID := "txn-" + uuid.New().String()
		err := testPoolExec(ctx, t, `UPDATE payments SET gateway_txn_id = $1 WHERE id = $2`, txnID, p.ID)
		require.NoError(t, err)

		got, err := repo.GetByGatewayTxnID(ctx, txnID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, txnID, got.GatewayTxnID)
	})

	t.Run("returns ErrNotFound when none", func(t *testing.T) {
		repo := New(testPool)

		got, err := repo.GetByGatewayTxnID(context.Background(), "nonexistent-txn-id")
		require.ErrorIs(t, err, apperror.ErrNotFound)
		assert.Nil(t, got)
	})
}

func TestPostgresRepository_GetByGatewayTxnID_WithNullableFields(t *testing.T) {
	t.Run("returns payment with payment_method_id and payment_url", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		txnID := "txn-nullable-" + uuid.New().String()
		require.NoError(t, testPoolExec(ctx, t,
			`UPDATE payments SET gateway_txn_id = $1, payment_url = $2, payment_method_id = $3 WHERE id = $4`,
			txnID, "https://pay.example.com/nullable", "pm_nullable_test", p.ID))

		got, err := repo.GetByGatewayTxnID(ctx, txnID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "pm_nullable_test", got.PaymentMethodID)
		assert.Equal(t, "https://pay.example.com/nullable", got.PaymentURL)
	})
}

func TestPostgresRepository_UpdateStatus(t *testing.T) {
	t.Run("transitions status", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		err := repo.UpdateStatus(ctx, p.ID, domain.StatusCancelled, []domain.Status{domain.StatusPending})
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusCancelled, got.Status)
	})

	t.Run("returns conflict when from-status does not match", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := New(testPool)

		err := repo.UpdateStatus(
			context.Background(),
			p.ID,
			domain.StatusCancelled,
			[]domain.Status{domain.StatusFailed},
		)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_ClearPaymentURL(t *testing.T) {
	t.Run("clears an existing payment url", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		require.NoError(t, testPoolExec(ctx, t,
			`UPDATE payments SET payment_url = $1 WHERE id = $2`, "https://pay.example.com/session/abc123", p.ID))

		err := repo.ClearPaymentURL(ctx, p.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Empty(t, got.PaymentURL)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("GetByID", func(t *testing.T) {
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetByGatewayTxnID", func(t *testing.T) {
		_, err := repo.GetByGatewayTxnID(cancelledCtx, "nonexistent")
		assert.Error(t, err)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		err := repo.UpdateStatus(
			cancelledCtx,
			uuid.New(),
			domain.StatusCancelled,
			[]domain.Status{domain.StatusPending},
		)
		assert.Error(t, err)
	})

	t.Run("ClearPaymentURL", func(t *testing.T) {
		err := repo.ClearPaymentURL(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})
}

func testPoolExec(ctx context.Context, t *testing.T, sql string, args ...any) error {
	t.Helper()
	_, err := testPool.Exec(ctx, sql, args...)
	return err
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

func seedOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD')`,
		id, userID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, id) })
	return id
}

// seedPayment inserts a payment row directly: webhook owns no Create method
// of its own (only charge does), so its tests seed through SQL.
func seedPayment(t *testing.T, orderID uuid.UUID) *domain.Payment {
	t.Helper()
	p := &domain.Payment{OrderID: orderID, Amount: money.New(1000, "USD"), Status: domain.StatusPending}
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO payments (order_id, amount, currency, status, method)
		 VALUES ($1, $2, $3, $4, 'card')
		 RETURNING id, created_at, updated_at`,
		p.OrderID, p.Amount.Amount, p.Amount.Currency, p.Status,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payments WHERE id = $1`, p.ID) })
	return p
}
