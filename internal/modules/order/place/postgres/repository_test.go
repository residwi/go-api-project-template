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
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_order")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates order with correct fields", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		o := newOrder(userID)
		err := repo.Create(context.Background(), o)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, o.ID)
		assert.Equal(t, domain.StatusAwaitingPayment, o.Status)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, o.ID) })
	})

	t.Run("returns conflict on duplicate idempotency key", func(t *testing.T) {
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := New(testPool)

		dup := newOrder(userID)
		dup.IdempotencyKey = o.IdempotencyKey
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_Create_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		o := newOrder(userID)
		err := repo.Create(ctx, o)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_CreateItems(t *testing.T) {
	t.Run("inserts all order items", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		o := seedOrder(t, userID)
		repo := New(testPool)
		ctx := context.Background()

		items := []domain.Item{
			{
				OrderID:     o.ID,
				ProductID:   productID,
				ProductName: "Widget",
				Price:       money.New(1000, "USD"),
				Quantity:    2,
				Subtotal:    money.New(2000, "USD"),
			},
		}
		err := repo.CreateItems(ctx, items)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, items[0].ID)

		got, err := repo.ListItemsByOrderID(ctx, o.ID)
		require.NoError(t, err)
		assert.Len(t, got, 1)
		assert.Equal(t, productID, got[0].ProductID)
		// order_items has no currency column: drop the join to orders and these read
		// back as money.New(n, ""), which refuses to Add into the order's total.
		assert.Equal(t, money.New(1000, "USD"), got[0].Price)
		assert.Equal(t, money.New(2000, "USD"), got[0].Subtotal)
	})
}

func TestPostgresRepository_CreateItems_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		userID := seedUser(t)
		o := seedOrder(t, userID)
		productID := seedProduct(t)
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		items := []domain.Item{
			{
				OrderID:     o.ID,
				ProductID:   productID,
				ProductName: "Widget",
				Price:       money.New(1000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(1000, "USD"),
			},
		}
		err := repo.CreateItems(ctx, items)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetByUserIDAndIdempotencyKey(t *testing.T) {
	t.Run("returns existing order", func(t *testing.T) {
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := New(testPool)

		got, err := repo.GetByUserIDAndIdempotencyKey(context.Background(), userID, o.IdempotencyKey)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, o.ID, got.ID)
	})

	t.Run("returns ErrNotFound when not found", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		got, err := repo.GetByUserIDAndIdempotencyKey(context.Background(), userID, "nonexistent-key")
		require.ErrorIs(t, err, apperror.ErrNotFound)
		assert.Nil(t, got)
	})

	// This looks up BY idempotency_key, so that column stays non-NULL and the
	// NULL-scan path is reached through request_hash and notes instead.
	t.Run("returns empty strings for request_hash and notes when NULL", func(t *testing.T) {
		userID := seedUser(t)
		ctx := context.Background()

		orderID := uuid.New()
		key := "key-" + orderID.String()
		_, err := testPool.Exec(
			ctx,
			`INSERT INTO orders (id, user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount, currency, notes)
			VALUES ($1, $2, $3, NULL, 'awaiting_payment', 1000, 0, 1000, 'USD', NULL)`,
			orderID,
			userID,
			key,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID) })

		repo := New(testPool)
		got, err := repo.GetByUserIDAndIdempotencyKey(ctx, userID, key)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, key, got.IdempotencyKey)
		assert.Empty(t, got.RequestHash)
		assert.Empty(t, got.Notes)
	})
}

func TestPostgresRepository_GetByUserIDAndIdempotencyKey_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetByUserIDAndIdempotencyKey(ctx, uuid.New(), "key")
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListItemsByOrderID(t *testing.T) {
	t.Run("returns all items for order", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		o := seedOrder(t, userID)
		repo := New(testPool)
		ctx := context.Background()

		items := []domain.Item{
			{
				OrderID:     o.ID,
				ProductID:   productID,
				ProductName: "Widget",
				Price:       money.New(500, "USD"),
				Quantity:    1,
				Subtotal:    money.New(500, "USD"),
			},
		}
		require.NoError(t, repo.CreateItems(ctx, items))

		got, err := repo.ListItemsByOrderID(ctx, o.ID)
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("returns empty slice for order with no items", func(t *testing.T) {
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := New(testPool)

		got, err := repo.ListItemsByOrderID(context.Background(), o.ID)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestPostgresRepository_ListItemsByOrderID_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.ListItemsByOrderID(ctx, uuid.New())
		assert.Error(t, err)
	})
}

// UpdateTotals had no repository-level test before this move -- only a mocked
// service-level one -- even though it finalizes the coupon discount on a real
// row. Added here since place is where it now lives.
func TestPostgresRepository_UpdateTotals(t *testing.T) {
	t.Run("updates discount and total on the order row", func(t *testing.T) {
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := New(testPool)

		err := repo.UpdateTotals(context.Background(), o.ID, 1000, 4000)
		require.NoError(t, err)

		got, err := repo.GetByUserIDAndIdempotencyKey(context.Background(), userID, o.IdempotencyKey)
		require.NoError(t, err)
		assert.Equal(t, money.New(1000, "USD"), got.Discount)
		assert.Equal(t, money.New(4000, "USD"), got.Total)
	})

	t.Run("returns not found for a missing order", func(t *testing.T) {
		repo := New(testPool)

		err := repo.UpdateTotals(context.Background(), uuid.New(), 0, 0)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD')`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })
	return id
}

func newOrder(userID uuid.UUID) *domain.Order {
	key := uuid.New().String()
	return &domain.Order{
		UserID:         userID,
		IdempotencyKey: key,
		RequestHash:    "hash-" + key,
		Status:         domain.StatusAwaitingPayment,
		Subtotal:       money.New(1000, "USD"),
		Discount:       money.New(0, "USD"),
		Total:          money.New(1000, "USD"),
	}
}

func seedOrder(t *testing.T, userID uuid.UUID) *domain.Order {
	t.Helper()
	repo := New(testPool)
	o := newOrder(userID)
	require.NoError(t, repo.Create(context.Background(), o))
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, o.ID) })
	return o
}
