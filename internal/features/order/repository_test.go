package order_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_order")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES ($1, $2, 'x', 'A', 'B')`,
		id, id.String()+"@test.com",
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id) })
	return id
}

func seedProduct(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO products (id, name, slug, description, price, currency, stock_quantity)
		 VALUES ($1, 'Product', $2, 'desc', 1000, 'USD', 10)`,
		id, "slug-"+id.String(),
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM products WHERE id = $1`, id) })
	return id
}

func newOrder(userID uuid.UUID) *order.Order {
	key := uuid.New().String()
	return &order.Order{
		UserID:         userID,
		IdempotencyKey: key,
		RequestHash:    "hash-" + key,
		Status:         order.StatusAwaitingPayment,
		SubtotalAmount: 1000,
		DiscountAmount: 0,
		TotalAmount:    1000,
		Currency:       "USD",
	}
}

func seedOrder(t *testing.T, userID uuid.UUID) *order.Order {
	t.Helper()
	repo := order.NewPostgresRepository(testPool)
	o := newOrder(userID)
	require.NoError(t, repo.Create(context.Background(), o))
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, o.ID) })
	return o
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates order with correct fields", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := order.NewPostgresRepository(testPool)

		o := newOrder(userID)
		err := repo.Create(context.Background(), o)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, o.ID)
		assert.Equal(t, order.StatusAwaitingPayment, o.Status)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, o.ID) })
	})

	t.Run("returns conflict on duplicate idempotency key", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)

		dup := newOrder(userID)
		dup.IdempotencyKey = o.IdempotencyKey
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestPostgresRepository_CreateItems(t *testing.T) {
	t.Run("inserts all order items", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		o := seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)
		ctx := context.Background()

		items := []order.Item{
			{OrderID: o.ID, ProductID: productID, ProductName: "Widget", Price: 1000, Quantity: 2, Subtotal: 2000},
		}
		err := repo.CreateItems(ctx, items)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, items[0].ID)

		got, err := repo.ListItemsByOrderID(ctx, o.ID)
		require.NoError(t, err)
		assert.Len(t, got, 1)
		assert.Equal(t, productID, got[0].ProductID)
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns order", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)

		got, err := repo.GetByID(context.Background(), o.ID)
		require.NoError(t, err)
		assert.Equal(t, o.ID, got.ID)
		assert.Equal(t, order.StatusAwaitingPayment, got.Status)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_GetByUserIDAndIdempotencyKey(t *testing.T) {
	t.Run("returns existing order", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)

		got, err := repo.GetByUserIDAndIdempotencyKey(context.Background(), userID, o.IdempotencyKey)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, o.ID, got.ID)
	})

	t.Run("returns ErrNotFound when not found", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := order.NewPostgresRepository(testPool)

		got, err := repo.GetByUserIDAndIdempotencyKey(context.Background(), userID, "nonexistent-key")
		require.ErrorIs(t, err, core.ErrNotFound)
		assert.Nil(t, got)
	})
}

func TestPostgresRepository_ListByUser(t *testing.T) {
	t.Run("returns paginated cursor results", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		for range 3 {
			seedOrder(t, userID)
		}
		repo := order.NewPostgresRepository(testPool)

		orders, err := repo.ListByUser(context.Background(), userID, core.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Len(t, orders, 3)
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		for range 4 {
			seedOrder(t, userID)
		}
		repo := order.NewPostgresRepository(testPool)

		page1, err := repo.ListByUser(context.Background(), userID, core.CursorPage{Limit: 2})
		require.NoError(t, err)
		require.Len(t, page1, 3) // limit+1 for hasMore

		last := page1[1]
		cursor := core.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())

		page2, err := repo.ListByUser(context.Background(), userID, core.CursorPage{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2)
		for _, o := range page2 {
			assert.NotEqual(t, page1[0].ID, o.ID)
			assert.NotEqual(t, page1[1].ID, o.ID)
		}
	})

	t.Run("returns empty for user with no orders", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := order.NewPostgresRepository(testPool)

		orders, err := repo.ListByUser(context.Background(), userID, core.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Empty(t, orders)
	})
}

func TestPostgresRepository_ListAdmin_NoFilter(t *testing.T) {
	t.Run("returns all orders without status filter", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)

		orders, total, err := repo.ListAdmin(context.Background(), order.AdminListParams{
			Page: 1, PageSize: 50,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, orders)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns offset-paginated results with status filter", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)

		orders, total, err := repo.ListAdmin(context.Background(), order.AdminListParams{
			Page: 1, PageSize: 10, Status: "awaiting_payment",
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, orders)
	})
}

func TestPostgresRepository_UpdateStatus(t *testing.T) {
	t.Run("transitions to new status", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)

		err := repo.UpdateStatus(context.Background(), o.ID, order.StatusAwaitingPayment, order.StatusPaymentProcessing)
		require.NoError(t, err)

		got, _ := repo.GetByID(context.Background(), o.ID)
		assert.Equal(t, order.StatusPaymentProcessing, got.Status)
	})

	t.Run("returns conflict when from-status does not match", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)

		// Order is awaiting_payment, try to transition from paid (wrong from-status)
		err := repo.UpdateStatus(context.Background(), o.ID, order.StatusPaid, order.StatusProcessing)
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestPostgresRepository_UpdateStatusMulti(t *testing.T) {
	t.Run("updates order matching any of the from-statuses", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)

		err := repo.UpdateStatusMulti(context.Background(), o.ID, order.StatusExpired,
			[]order.Status{order.StatusAwaitingPayment, order.StatusCancelled})
		require.NoError(t, err)

		got, _ := repo.GetByID(context.Background(), o.ID)
		assert.Equal(t, order.StatusExpired, got.Status)
	})
}

func TestPostgresRepository_ListItemsByOrderID(t *testing.T) {
	t.Run("returns all items for order", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		o := seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)
		ctx := context.Background()

		items := []order.Item{
			{OrderID: o.ID, ProductID: productID, ProductName: "Widget", Price: 500, Quantity: 1, Subtotal: 500},
		}
		require.NoError(t, repo.CreateItems(ctx, items))

		got, err := repo.ListItemsByOrderID(ctx, o.ID)
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("returns empty slice for order with no items", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := order.NewPostgresRepository(testPool)

		got, err := repo.ListItemsByOrderID(context.Background(), o.ID)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestPostgresRepository_GetExpiredOrders(t *testing.T) {
	t.Run("returns awaiting_payment orders older than 30 minutes", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := order.NewPostgresRepository(testPool)
		ctx := context.Background()

		// Insert an order backdated by 2 hours
		oldOrderID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency, created_at)
			 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD', NOW() - INTERVAL '2 hours')`,
			oldOrderID, userID,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, oldOrderID) })

		orders, err := repo.GetExpiredOrders(ctx, 10)
		require.NoError(t, err)

		var found bool
		for _, o := range orders {
			if o.ID == oldOrderID {
				found = true
				break
			}
		}
		assert.True(t, found, "expected old awaiting_payment order to appear in expired orders")
	})

	t.Run("does not return recent orders", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		o := seedOrder(t, userID) // created now — not expired
		repo := order.NewPostgresRepository(testPool)

		orders, err := repo.GetExpiredOrders(context.Background(), 100)
		require.NoError(t, err)

		for _, got := range orders {
			assert.NotEqual(t, o.ID, got.ID)
		}
	})
}

func TestPostgresRepository_Create_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		o := newOrder(userID)
		err := repo.Create(ctx, o)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_CreateItems_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		o := seedOrder(t, userID)
		productID := seedProduct(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		items := []order.Item{
			{OrderID: o.ID, ProductID: productID, ProductName: "Widget", Price: 1000, Quantity: 1, Subtotal: 1000},
		}
		err := repo.CreateItems(ctx, items)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetByID_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetByID(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetByUserIDAndIdempotencyKey_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetByUserIDAndIdempotencyKey(ctx, uuid.New(), "key")
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListByUser_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.ListByUser(ctx, uuid.New(), core.CursorPage{Limit: 10})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListAdmin_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := repo.ListAdmin(ctx, order.AdminListParams{Page: 1, PageSize: 10})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_UpdateStatus_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.UpdateStatus(ctx, uuid.New(), order.StatusAwaitingPayment, order.StatusPaid)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_UpdateStatusMulti_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := repo.UpdateStatusMulti(ctx, uuid.New(), order.StatusPaid,
			[]order.Status{order.StatusAwaitingPayment})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListItemsByOrderID_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.ListItemsByOrderID(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetExpiredOrders_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetExpiredOrders(ctx, 10)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetStaleProcessingOrders_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		setup(t)
		repo := order.NewPostgresRepository(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetStaleProcessingOrders(ctx, 30*time.Minute, 10)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_GetStaleProcessingOrders(t *testing.T) {
	t.Run("returns orders stuck in payment_processing beyond threshold", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		repo := order.NewPostgresRepository(testPool)
		ctx := context.Background()

		// Insert a stale processing order
		staleID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency, updated_at)
			 VALUES ($1, $2, 'payment_processing', 1000, 0, 1000, 'USD', NOW() - INTERVAL '1 hour')`,
			staleID, userID,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM orders WHERE id = $1`, staleID) })

		orders, err := repo.GetStaleProcessingOrders(ctx, 30*time.Minute, 10)
		require.NoError(t, err)

		var found bool
		for _, o := range orders {
			if o.ID == staleID {
				found = true
				break
			}
		}
		assert.True(t, found, "expected stale processing order to be returned")
	})
}
