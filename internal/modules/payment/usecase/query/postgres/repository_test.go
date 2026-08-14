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
	"github.com/residwi/go-api-project-template/internal/modules/payment/usecase/query"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
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
			`UPDATE payments SET payment_method_id = $1 WHERE id = $2`, "pm_get_test", p.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "pm_get_test", got.PaymentMethodID)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns paginated results", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		seedPayment(t, orderID)
		repo := New(testPool)

		payments, total, err := repo.ListAdmin(context.Background(), query.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10},
			OrderID:    orderID.String(),
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, payments)
	})
}

func TestPostgresRepository_ListAdmin_WithNullableFields(t *testing.T) {
	t.Run("returns payments with payment_method_id and gateway_txn_id", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1, gateway_txn_id = $2 WHERE id = $3`,
			"pm_admin_list", "txn-admin-list", p.ID)
		require.NoError(t, err)

		payments, total, err := repo.ListAdmin(ctx, query.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 100},
			OrderID:    orderID.String(),
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		var found bool
		for _, pay := range payments {
			if pay.ID == p.ID {
				assert.Equal(t, "pm_admin_list", pay.PaymentMethodID)
				assert.Equal(t, "txn-admin-list", pay.GatewayTxnID)
				found = true
			}
		}
		assert.True(t, found)
	})
}

func TestPostgresRepository_ListAdmin_Filters(t *testing.T) {
	t.Run("filters by status", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		seedPayment(t, orderID)
		repo := New(testPool)

		payments, total, err := repo.ListAdmin(context.Background(), query.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10},
			OrderID:    orderID.String(),
			Status:     "pending",
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, got := range payments {
			assert.Equal(t, domain.StatusPending, got.Status)
		}
	})

	t.Run("filters by order ID", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		seedPayment(t, orderID)
		repo := New(testPool)

		payments, total, err := repo.ListAdmin(context.Background(), query.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10},
			OrderID:    orderID.String(),
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, p := range payments {
			assert.Equal(t, orderID, p.OrderID)
		}
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

	t.Run("ListAdmin", func(t *testing.T) {
		_, _, err := repo.ListAdmin(
			cancelledCtx,
			query.AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}},
		)
		assert.Error(t, err)
	})
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

// seedPayment inserts a payment row directly: query owns no write method, so
// its tests seed through SQL rather than a repository call.
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
