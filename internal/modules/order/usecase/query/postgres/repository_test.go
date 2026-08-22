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
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/query"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_order")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns order", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID, money.New(1000, "USD"))
		repo := New(testPool)

		got, err := repo.GetByID(context.Background(), orderID)
		require.NoError(t, err)
		assert.Equal(t, orderID, got.ID)
		assert.Equal(t, domain.StatusAwaitingPayment, got.Status)
	})

	// Three amounts, one shared currency column, and amountColumns.assignTo is the
	// only place that fan-out happens. Two mutations to it once survived the whole
	// suite: denominating Subtotal and Discount as money.New(n, ""), and swapping
	// subtotal with discount.
	t.Run("denominates all three amounts from the row's single currency", func(t *testing.T) {
		userID := seedUser(t)
		ctx := context.Background()

		orderID := uuid.New()
		_, err := testPool.Exec(
			ctx,
			`INSERT INTO orders (id, user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount, currency)
			 VALUES ($1, $2, $3, 'awaiting_payment', 9500, 1500, 8000, 'IDR')`,
			orderID,
			userID,
			uuid.New().String(),
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID) })

		repo := New(testPool)
		got, err := repo.GetByID(ctx, orderID)
		require.NoError(t, err)

		assert.Equal(t, money.New(9500, "IDR"), got.Subtotal, "subtotal must carry the row's currency")
		assert.Equal(t, money.New(1500, "IDR"), got.Discount, "discount must carry the row's currency")
		assert.Equal(t, money.New(8000, "IDR"), got.Total, "total must carry the row's currency")
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)
		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	// Create always writes non-empty values into these three nullable columns, so
	// only a row seeded outside the repository reaches the NULL path -- where
	// scanning into a plain Go string is a runtime error.
	t.Run("returns empty strings when idempotency_key, request_hash, and notes are NULL", func(t *testing.T) {
		userID := seedUser(t)
		ctx := context.Background()

		orderID := uuid.New()
		_, err := testPool.Exec(
			ctx,
			`INSERT INTO orders (id, user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount, currency, notes)
			VALUES ($1, $2, NULL, NULL, 'awaiting_payment', 1000, 0, 1000, 'USD', NULL)`,
			orderID,
			userID,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID) })

		repo := New(testPool)
		got, err := repo.GetByID(ctx, orderID)
		require.NoError(t, err)
		assert.Empty(t, got.IdempotencyKey)
		assert.Empty(t, got.RequestHash)
		assert.Empty(t, got.Notes)
	})
}

func TestPostgresRepository_GetByID_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.GetByID(ctx, uuid.New())
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListByUser(t *testing.T) {
	t.Run("returns paginated cursor results", func(t *testing.T) {
		userID := seedUser(t)
		for range 3 {
			seedOrder(t, userID, money.New(1000, "USD"))
		}
		repo := New(testPool)

		orders, err := repo.ListByUser(context.Background(), userID, paging.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Len(t, orders, 3)
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		userID := seedUser(t)
		for range 4 {
			seedOrder(t, userID, money.New(1000, "USD"))
		}
		repo := New(testPool)

		page1, err := repo.ListByUser(context.Background(), userID, paging.CursorPage{Limit: 2})
		require.NoError(t, err)
		require.Len(t, page1, 3) // limit+1 for hasMore

		last := page1[1]
		cursor := paging.EncodeCursor(last.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), last.ID.String())

		page2, err := repo.ListByUser(context.Background(), userID, paging.CursorPage{Cursor: cursor, Limit: 2})
		require.NoError(t, err)
		assert.NotEmpty(t, page2)
		for _, o := range page2 {
			assert.NotEqual(t, page1[0].ID, o.ID)
			assert.NotEqual(t, page1[1].ID, o.ID)
		}
	})

	t.Run("returns empty for user with no orders", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(testPool)

		orders, err := repo.ListByUser(context.Background(), userID, paging.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Empty(t, orders)
	})

	// scanOrder nil-checks idempotency_key but scans notes into a plain string, so
	// a NULL notes row is what reaches that bug through this reader.
	t.Run("returns empty notes when NULL", func(t *testing.T) {
		userID := seedUser(t)
		ctx := context.Background()

		orderID := uuid.New()
		_, err := testPool.Exec(
			ctx,
			`INSERT INTO orders (id, user_id, idempotency_key, request_hash, status, subtotal_amount, discount_amount, total_amount, currency, notes)
			VALUES ($1, $2, NULL, NULL, 'awaiting_payment', 1000, 0, 1000, 'USD', NULL)`,
			orderID,
			userID,
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID) })

		repo := New(testPool)
		orders, err := repo.ListByUser(ctx, userID, paging.CursorPage{Limit: 10})
		require.NoError(t, err)
		require.Len(t, orders, 1)
		assert.Empty(t, orders[0].IdempotencyKey)
		assert.Empty(t, orders[0].Notes)
	})
}

func TestPostgresRepository_ListByUser_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, err := repo.ListByUser(ctx, uuid.New(), paging.CursorPage{Limit: 10})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListAdmin_NoFilter(t *testing.T) {
	t.Run("returns all orders without status filter", func(t *testing.T) {
		userID := seedUser(t)
		seedOrder(t, userID, money.New(1000, "USD"))
		repo := New(testPool)

		orders, total, err := repo.ListAdmin(context.Background(), query.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 50},
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, orders)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns offset-paginated results with status filter", func(t *testing.T) {
		userID := seedUser(t)
		seedOrder(t, userID, money.New(1000, "USD"))
		repo := New(testPool)

		orders, total, err := repo.ListAdmin(context.Background(), query.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}, Status: "awaiting_payment",
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, orders)
	})
}

func TestPostgresRepository_ListAdmin_CancelledContext(t *testing.T) {
	t.Run("returns error on cancelled context", func(t *testing.T) {
		repo := New(testPool)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := repo.ListAdmin(ctx, query.AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}})
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListItemsByOrderID(t *testing.T) {
	t.Run("returns all items for order", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID, money.New(500, "USD"))
		ctx := context.Background()

		_, err := testPool.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal)
			 VALUES ($1, $2, 'Widget', 500, 1, 500)`, orderID, productID)
		require.NoError(t, err)

		repo := New(testPool)
		got, err := repo.ListItemsByOrderID(ctx, orderID)
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("returns empty slice for order with no items", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID, money.New(1000, "USD"))
		repo := New(testPool)

		got, err := repo.ListItemsByOrderID(context.Background(), orderID)
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

func TestPostgresRepository_HasDeliveredOrder(t *testing.T) {
	repo := New(testPool)
	ctx := context.Background()

	seedOrderItem := func(t *testing.T, userID, productID uuid.UUID, status domain.Status) uuid.UUID {
		t.Helper()
		orderID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
			 VALUES ($1, $2, $3, 1000, 0, 1000, 'USD')`,
			orderID, userID, string(status),
		)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID) })
		_, err = testPool.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal)
			 VALUES ($1, $2, 'Product', 1000, 1, 1000)`,
			orderID, productID,
		)
		require.NoError(t, err)
		return orderID
	}

	t.Run("true when the order is delivered and contains the product", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrderItem(t, userID, productID, domain.StatusDelivered)

		ok, err := repo.HasDeliveredOrder(ctx, query.DeliveredPurchaseParams{
			UserID:    userID,
			OrderID:   orderID,
			ProductID: productID,
		})

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("false when the matching order is not delivered", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrderItem(t, userID, productID, domain.StatusPaid)

		ok, err := repo.HasDeliveredOrder(ctx, query.DeliveredPurchaseParams{
			UserID:    userID,
			OrderID:   orderID,
			ProductID: productID,
		})

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("false when the user has no such order", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)

		ok, err := repo.HasDeliveredOrder(ctx, query.DeliveredPurchaseParams{
			UserID:    userID,
			OrderID:   uuid.New(),
			ProductID: productID,
		})

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("false when the delivered order belongs to another user", func(t *testing.T) {
		buyer := seedUser(t)
		other := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrderItem(t, buyer, productID, domain.StatusDelivered)

		ok, err := repo.HasDeliveredOrder(ctx, query.DeliveredPurchaseParams{
			UserID:    other,
			OrderID:   orderID,
			ProductID: productID,
		})

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("false when the orderID is not the order that delivered the product", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		seedOrderItem(t, userID, productID, domain.StatusDelivered)

		ok, err := repo.HasDeliveredOrder(ctx, query.DeliveredPurchaseParams{
			UserID:    userID,
			OrderID:   uuid.New(),
			ProductID: productID,
		})

		require.NoError(t, err)
		assert.False(t, ok)
	})
}

// TestUseCase_GetSnapshot runs against the real repository rather than a
// mock: checkout's retry-payment ownership check and its ChargeRequest.OrderID
// both come from GetSnapshot's ID and UserID, and a mock that hands back a
// fully-populated contract.Order can't catch GetSnapshot forgetting to copy
// them from the row it just read -- which is exactly what shipped.
func TestUseCase_GetSnapshot(t *testing.T) {
	userID := seedUser(t)
	orderID := seedOrder(t, userID, money.New(4200, "USD"))

	r := query.New(New(testPool))
	got, err := r.GetSnapshot(context.Background(), orderID)

	require.NoError(t, err)
	assert.Equal(t, orderID, got.ID)
	assert.Equal(t, userID, got.UserID)
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

// seedOrder inserts a minimal awaiting_payment order: raw SQL, not place's
// repository, so this package never imports a sibling slice.
func seedOrder(t *testing.T, userID uuid.UUID, total money.Money) uuid.UUID {
	t.Helper()
	var orderID uuid.UUID
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO orders (user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'awaiting_payment', $3, 0, $3, $4) RETURNING id`,
		userID, uuid.New().String(), total.Amount, total.Currency,
	).Scan(&orderID)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, orderID) })
	return orderID
}
