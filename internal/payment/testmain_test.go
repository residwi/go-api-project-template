package payment_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/payment"
	"github.com/residwi/go-api-project-template/internal/payment/postgres"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// testPool backs this package's service-level integration test
// (jobs_integration_test.go). It is separate from the postgres subpackage's
// own container/DB to avoid two test binaries racing on the same database.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_payment_svc")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
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

func seedPayment(t *testing.T, orderID uuid.UUID) *payment.Payment {
	t.Helper()
	repo := postgres.New(testPool)
	p := &payment.Payment{
		OrderID: orderID,
		Amount:  money.New(1000, "USD"),
		Status:  payment.StatusPending,
		Method:  "card",
	}
	err := repo.Create(context.Background(), p)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payments WHERE id = $1`, p.ID) })
	return p
}
