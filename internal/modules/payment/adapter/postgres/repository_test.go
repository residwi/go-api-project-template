package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_payment")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates payment with correct fields", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})

		p := &domain.Payment{
			OrderID: orderID,
			Amount:  money.New(1000, "USD"),
			Status:  domain.StatusPending,
			Method:  "card",
		}
		err := repo.Create(context.Background(), p)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payments WHERE id = $1`, p.ID) })

		assert.NotEqual(t, uuid.Nil, p.ID)
		assert.Equal(t, orderID, p.OrderID)
		// Comparing the whole Money proves the write put the amount and the denomination
		// in the columns that belong to each other.
		assert.Equal(t, money.New(1000, "USD"), p.Amount)
		assert.Equal(t, domain.StatusPending, p.Status)
		assert.Equal(t, "card", p.Method)
		assert.False(t, p.CreatedAt.IsZero())
		assert.False(t, p.UpdatedAt.IsZero())
	})
}

func TestPostgresRepository_GetActiveByOrderID(t *testing.T) {
	t.Run("returns active payment for order", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)

		got, err := repo.GetActiveByOrderID(context.Background(), orderID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, p.ID, got.ID)
	})

	t.Run("returns ErrNotFound when none", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetActiveByOrderID(context.Background(), orderID)
		require.ErrorIs(t, err, errs.ErrNotFound)
		assert.Nil(t, got)
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns payment", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, p.OrderID, got.OrderID)
		assert.Equal(t, p.Amount, got.Amount)
		assert.Equal(t, p.Status, got.Status)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestPostgresRepository_MarkPaid(t *testing.T) {
	t.Run("marks payment as paid", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		err := repo.MarkPaid(ctx, p.ID, []domain.Status{domain.StatusPending})
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusSuccess, got.Status)
		assert.NotNil(t, got.PaidAt)
	})

	t.Run("returns conflict when status does not match", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)

		err := repo.MarkPaid(context.Background(), p.ID, []domain.Status{domain.StatusFailed})
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestPostgresRepository_UpdateStatus(t *testing.T) {
	t.Run("transitions status", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		err := repo.UpdateStatus(ctx, p.ID, domain.StatusRequiresReview, []domain.Status{domain.StatusPending})
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusRequiresReview, got.Status)
	})

	t.Run("returns conflict when from-status does not match", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)

		err := repo.UpdateStatus(
			context.Background(),
			p.ID,
			domain.StatusRequiresReview,
			[]domain.Status{domain.StatusFailed},
		)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestPostgresRepository_GetActiveByOrderID_WithNullableFields(t *testing.T) {
	t.Run("returns active payment with payment_method_id, payment_url, and gateway_txn_id", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		require.NoError(t, repo.UpdateGateway(ctx, p.ID, "txn-charge-active-test", []byte(`{}`)))
		require.NoError(t, repo.UpdatePaymentURL(ctx, p.ID, "https://pay.example.com/active"))
		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1 WHERE id = $2`, "pm_charge_active_test", p.ID)
		require.NoError(t, err)

		got, err := repo.GetActiveByOrderID(ctx, orderID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "pm_charge_active_test", got.PaymentMethodID)
		assert.Equal(t, "https://pay.example.com/active", got.PaymentURL)
		assert.Equal(t, "txn-charge-active-test", got.GatewayTxnID)
	})
}

func TestPostgresRepository_UpdateGateway(t *testing.T) {
	t.Run("updates gateway txn id and response", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		txnID := "gw-txn-charge-" + uuid.New().String()
		response := []byte(`{"code":200}`)
		err := repo.UpdateGateway(ctx, p.ID, txnID, response)
		require.NoError(t, err)

		got, err := repo.GetActiveByOrderID(ctx, orderID)
		require.NoError(t, err)
		assert.Equal(t, txnID, got.GatewayTxnID)
		assert.JSONEq(t, string(response), string(got.GatewayResponse))
	})
}

func TestPostgresRepository_UpdatePaymentURL(t *testing.T) {
	t.Run("updates payment url", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		url := "https://pay.example.com/session/abc123"
		err := repo.UpdatePaymentURL(ctx, p.ID, url)
		require.NoError(t, err)

		got, err := repo.GetActiveByOrderID(ctx, orderID)
		require.NoError(t, err)
		assert.Equal(t, url, got.PaymentURL)
	})
}

func TestPostgresRepository_GetByID_WithNullableFields(t *testing.T) {
	t.Run("returns payment with payment_method_id", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1 WHERE id = $2`, "pm_get_test", p.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "pm_get_test", got.PaymentMethodID)
	})
}

func TestPostgresRepository_GetByGatewayTxnID(t *testing.T) {
	t.Run("returns payment by txn id", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		txnID := "txn-" + uuid.New().String()
		_, err := testPool.Exec(ctx, `UPDATE payments SET gateway_txn_id = $1 WHERE id = $2`, txnID, p.ID)
		require.NoError(t, err)

		got, err := repo.GetByGatewayTxnID(ctx, txnID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, txnID, got.GatewayTxnID)
	})

	t.Run("returns ErrNotFound when none", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByGatewayTxnID(context.Background(), "nonexistent-txn-id")
		require.ErrorIs(t, err, errs.ErrNotFound)
		assert.Nil(t, got)
	})
}

func TestPostgresRepository_GetByGatewayTxnID_WithNullableFields(t *testing.T) {
	t.Run("returns payment with payment_method_id and payment_url", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		txnID := "txn-nullable-" + uuid.New().String()
		_, err := testPool.Exec(ctx,
			`UPDATE payments SET gateway_txn_id = $1, payment_url = $2, payment_method_id = $3 WHERE id = $4`,
			txnID, "https://pay.example.com/nullable", "pm_nullable_test", p.ID)
		require.NoError(t, err)

		got, err := repo.GetByGatewayTxnID(ctx, txnID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "pm_nullable_test", got.PaymentMethodID)
		assert.Equal(t, "https://pay.example.com/nullable", got.PaymentURL)
	})
}

func TestPostgresRepository_ClearPaymentURL(t *testing.T) {
	t.Run("clears an existing payment url", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_url = $1 WHERE id = $2`, "https://pay.example.com/session/abc123", p.ID)
		require.NoError(t, err)

		err = repo.ClearPaymentURL(ctx, p.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Empty(t, got.PaymentURL)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns paginated results", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
		seedPayment(t, repo, orderID)

		payments, total, err := repo.ListAdmin(context.Background(), payment.AdminListParams{
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
		repo := New(database.DB{Primary: testPool})
		p := seedPayment(t, repo, orderID)
		ctx := context.Background()

		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1, gateway_txn_id = $2 WHERE id = $3`,
			"pm_admin_list", "txn-admin-list", p.ID)
		require.NoError(t, err)

		payments, total, err := repo.ListAdmin(ctx, payment.AdminListParams{
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
		repo := New(database.DB{Primary: testPool})
		seedPayment(t, repo, orderID)

		payments, total, err := repo.ListAdmin(context.Background(), payment.AdminListParams{
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
		repo := New(database.DB{Primary: testPool})
		seedPayment(t, repo, orderID)

		payments, total, err := repo.ListAdmin(context.Background(), payment.AdminListParams{
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

	repo := New(database.DB{Primary: testPool})

	t.Run("Create", func(t *testing.T) {
		p := &domain.Payment{OrderID: uuid.New(), Amount: money.New(1000, "USD"), Status: domain.StatusPending}
		err := repo.Create(cancelledCtx, p)
		assert.Error(t, err)
	})

	t.Run("GetByID", func(t *testing.T) {
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetActiveByOrderID", func(t *testing.T) {
		_, err := repo.GetActiveByOrderID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetByGatewayTxnID", func(t *testing.T) {
		_, err := repo.GetByGatewayTxnID(cancelledCtx, "nonexistent")
		assert.Error(t, err)
	})

	t.Run("UpdateGateway", func(t *testing.T) {
		err := repo.UpdateGateway(cancelledCtx, uuid.New(), "txn", []byte(`{}`))
		assert.Error(t, err)
	})

	t.Run("UpdatePaymentURL", func(t *testing.T) {
		err := repo.UpdatePaymentURL(cancelledCtx, uuid.New(), "https://pay.example.com")
		assert.Error(t, err)
	})

	t.Run("ClearPaymentURL", func(t *testing.T) {
		err := repo.ClearPaymentURL(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("MarkPaid", func(t *testing.T) {
		err := repo.MarkPaid(cancelledCtx, uuid.New(), []domain.Status{domain.StatusPending})
		assert.Error(t, err)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		err := repo.UpdateStatus(
			cancelledCtx,
			uuid.New(),
			domain.StatusRequiresReview,
			[]domain.Status{domain.StatusPending},
		)
		assert.Error(t, err)
	})

	t.Run("ListAdmin", func(t *testing.T) {
		_, _, err := repo.ListAdmin(
			cancelledCtx,
			payment.AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}},
		)
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testutil.SeedUser(t, testPool)
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

// seedPayment goes through charge's own Create, since charge is the one slice
// that owns writing a brand new payment row.
func seedPayment(t *testing.T, repo *Repository, orderID uuid.UUID) *domain.Payment {
	t.Helper()
	p := &domain.Payment{
		OrderID: orderID,
		Amount:  money.New(1000, "USD"),
		Status:  domain.StatusPending,
		Method:  "card",
	}
	err := repo.Create(context.Background(), p)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payments WHERE id = $1`, p.ID) })
	return p
}
