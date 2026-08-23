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
	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_order")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates order with correct fields", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})

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
		repo := New(database.DB{Primary: testPool})

		dup := newOrder(userID)
		dup.IdempotencyKey = o.IdempotencyKey
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestPostgresRepository_CreateItems(t *testing.T) {
	t.Run("inserts all order items", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		o := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})
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

func TestPostgresRepository_GetByUserIDAndIdempotencyKey(t *testing.T) {
	t.Run("returns existing order", func(t *testing.T) {
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByUserIDAndIdempotencyKey(context.Background(), userID, o.IdempotencyKey)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, o.ID, got.ID)
	})

	t.Run("returns ErrNotFound when not found", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})

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

		repo := New(database.DB{Primary: testPool})
		got, err := repo.GetByUserIDAndIdempotencyKey(ctx, userID, key)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, key, got.IdempotencyKey)
		assert.Empty(t, got.RequestHash)
		assert.Empty(t, got.Notes)
	})
}

// UpdateTotals had no repository-level test before the slices were introduced --
// only a mocked service-level one -- even though it finalizes the coupon
// discount on a real row.
func TestPostgresRepository_UpdateTotals(t *testing.T) {
	t.Run("updates discount and total on the order row", func(t *testing.T) {
		userID := seedUser(t)
		o := seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})

		err := repo.UpdateTotals(context.Background(), o.ID, 1000, 4000)
		require.NoError(t, err)

		got, err := repo.GetByUserIDAndIdempotencyKey(context.Background(), userID, o.IdempotencyKey)
		require.NoError(t, err)
		assert.Equal(t, money.New(1000, "USD"), got.Discount)
		assert.Equal(t, money.New(4000, "USD"), got.Total)
	})

	t.Run("returns not found for a missing order", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		err := repo.UpdateTotals(context.Background(), uuid.New(), 0, 0)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns order", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID).ID
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByID(context.Background(), orderID)
		require.NoError(t, err)
		assert.Equal(t, orderID, got.ID)
		assert.Equal(t, domain.StatusAwaitingPayment, got.Status)
	})

	// Service.Snapshot copies ID and UserID straight off this row, and
	// checkout's retry-payment ownership check plus its ChargeRequest.OrderID
	// are both built from them. A mocked Snapshot test cannot prove the row
	// carries them -- only this can, and a version of this read that dropped
	// them is what shipped a dead endpoint once.
	t.Run("populates the ID and UserID that Snapshot projects", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID).ID
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByID(context.Background(), orderID)
		require.NoError(t, err)
		assert.Equal(t, orderID, got.ID)
		assert.Equal(t, userID, got.UserID)
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

		repo := New(database.DB{Primary: testPool})
		got, err := repo.GetByID(ctx, orderID)
		require.NoError(t, err)

		assert.Equal(t, money.New(9500, "IDR"), got.Subtotal, "subtotal must carry the row's currency")
		assert.Equal(t, money.New(1500, "IDR"), got.Discount, "discount must carry the row's currency")
		assert.Equal(t, money.New(8000, "IDR"), got.Total, "total must carry the row's currency")
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
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

		repo := New(database.DB{Primary: testPool})
		got, err := repo.GetByID(ctx, orderID)
		require.NoError(t, err)
		assert.Empty(t, got.IdempotencyKey)
		assert.Empty(t, got.RequestHash)
		assert.Empty(t, got.Notes)
	})
}

func TestPostgresRepository_ListByUser(t *testing.T) {
	t.Run("returns paginated cursor results", func(t *testing.T) {
		userID := seedUser(t)
		for range 3 {
			seedOrder(t, userID)
		}
		repo := New(database.DB{Primary: testPool})

		orders, err := repo.ListByUser(context.Background(), userID, paging.CursorPage{Limit: 10})
		require.NoError(t, err)
		assert.Len(t, orders, 3)
	})

	t.Run("cursor pagination returns next page", func(t *testing.T) {
		userID := seedUser(t)
		for range 4 {
			seedOrder(t, userID)
		}
		repo := New(database.DB{Primary: testPool})

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
		repo := New(database.DB{Primary: testPool})

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

		repo := New(database.DB{Primary: testPool})
		orders, err := repo.ListByUser(ctx, userID, paging.CursorPage{Limit: 10})
		require.NoError(t, err)
		require.Len(t, orders, 1)
		assert.Empty(t, orders[0].IdempotencyKey)
		assert.Empty(t, orders[0].Notes)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns all orders without status filter", func(t *testing.T) {
		userID := seedUser(t)
		seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})

		orders, total, err := repo.ListAdmin(context.Background(), order.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 50},
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, orders)
	})

	t.Run("returns offset-paginated results with status filter", func(t *testing.T) {
		userID := seedUser(t)
		seedOrder(t, userID)
		repo := New(database.DB{Primary: testPool})

		orders, total, err := repo.ListAdmin(context.Background(), order.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}, Status: "awaiting_payment",
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, orders)
	})
}

func TestPostgresRepository_ListItemsByOrderID(t *testing.T) {
	t.Run("returns all items for order", func(t *testing.T) {
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrder(t, userID).ID
		ctx := context.Background()

		_, err := testPool.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal)
			 VALUES ($1, $2, 'Widget', 500, 1, 500)`, orderID, productID)
		require.NoError(t, err)

		repo := New(database.DB{Primary: testPool})
		got, err := repo.ListItemsByOrderID(ctx, orderID)
		require.NoError(t, err)
		assert.Len(t, got, 1)
	})

	t.Run("returns empty slice for order with no items", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID).ID
		repo := New(database.DB{Primary: testPool})

		got, err := repo.ListItemsByOrderID(context.Background(), orderID)
		require.NoError(t, err)
		assert.Empty(t, got)
	})
}

func TestPostgresRepository_HasDeliveredOrder(t *testing.T) {
	repo := New(database.DB{Primary: testPool})
	ctx := context.Background()

	seedOrderItem := func(t *testing.T, userID, productID uuid.UUID, status domain.Status) uuid.UUID {
		t.Helper()
		orderID := seedOrderWith(t, userID, status, money.New(1000, "USD")).ID
		_, err := testPool.Exec(ctx,
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

		ok, err := repo.HasDeliveredOrder(ctx, order.DeliveredPurchaseParams{
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

		ok, err := repo.HasDeliveredOrder(ctx, order.DeliveredPurchaseParams{
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

		ok, err := repo.HasDeliveredOrder(ctx, order.DeliveredPurchaseParams{
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

		ok, err := repo.HasDeliveredOrder(ctx, order.DeliveredPurchaseParams{
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

		ok, err := repo.HasDeliveredOrder(ctx, order.DeliveredPurchaseParams{
			UserID:    userID,
			OrderID:   uuid.New(),
			ProductID: productID,
		})

		require.NoError(t, err)
		assert.False(t, ok)
	})
}

func TestPostgresRepository_GetExpiredOrders(t *testing.T) {
	t.Run("returns awaiting_payment orders older than 30 minutes", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		oldOrderID := uuid.New()
		// GetExpiredOrders sorts oldest-first with a LIMIT, and test_order is a
		// shared database that is never truncated (see the registry comment in
		// internal/testutil/testutil.go), so accumulated rows from earlier
		// runs sort ahead of this one. 100 years is arbitrary -- it only needs to
		// predate every row that has ever accumulated so this order is always
		// oldest and never crowded out of the LIMIT.
		_, err := testPool.Exec(
			ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency, created_at)
			 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD', NOW() - INTERVAL '100 years')`,
			oldOrderID,
			userID,
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
		userID := seedUser(t)
		orderID := seedOrder(t, userID).ID // created now -- not expired
		repo := New(database.DB{Primary: testPool})

		orders, err := repo.GetExpiredOrders(context.Background(), 100)
		require.NoError(t, err)

		for _, got := range orders {
			assert.NotEqual(t, orderID, got.ID)
		}
	})
}

func TestPostgresRepository_GetStaleProcessingOrders(t *testing.T) {
	t.Run("returns orders stuck in payment_processing beyond threshold", func(t *testing.T) {
		userID := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		staleID := uuid.New()
		// GetStaleProcessingOrders sorts oldest-first with a LIMIT, and
		// test_order is a shared database that is never truncated (see the
		// registry comment in internal/testutil/testutil.go), so
		// accumulated rows from earlier runs sort ahead of this one. 100 years
		// is arbitrary -- it only needs to predate every row that has ever
		// accumulated so this order is always oldest and never crowded out of
		// the LIMIT.
		_, err := testPool.Exec(
			ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency, updated_at)
			 VALUES ($1, $2, 'payment_processing', 1000, 0, 1000, 'USD', NOW() - INTERVAL '100 years')`,
			staleID,
			userID,
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

func TestPostgresRepository_Apply(t *testing.T) {
	t.Run("updates order matching any of the from-statuses", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID).ID
		repo := New(database.DB{Primary: testPool})

		err := repo.Apply(context.Background(), orderID, domain.Transition{
			To:   domain.StatusExpired,
			From: []domain.Status{domain.StatusAwaitingPayment, domain.StatusCancelled},
		})
		require.NoError(t, err)

		assert.Equal(t, domain.StatusExpired, statusOf(t, orderID))
	})

	t.Run("conflict when current status is not in from", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrderWith(t, userID, domain.StatusPaid, money.New(1000, "USD")).ID
		repo := New(database.DB{Primary: testPool})

		err := repo.Apply(context.Background(), orderID, domain.PaidTransition)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("sets the stock flags the transition carries", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrderWith(t, userID, domain.StatusPaymentProcessing, money.New(1000, "USD")).ID
		repo := New(database.DB{Primary: testPool})

		require.NoError(t, repo.Apply(context.Background(), orderID, domain.PaidTransition))

		deducted, reversed := stockFlagsOf(t, orderID)
		assert.True(t, deducted)
		assert.False(t, reversed)
	})
}

func TestPostgresRepository_UpdateStatus(t *testing.T) {
	t.Run("transitions to new status", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID).ID
		repo := New(database.DB{Primary: testPool})

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
		orderID := seedOrder(t, userID).ID
		repo := New(database.DB{Primary: testPool})

		// paid is the wrong from-status for an awaiting_payment order.
		err := repo.UpdateStatus(context.Background(), orderID, domain.StatusPaid, domain.StatusProcessing)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

// TestPostgresRepository_CancelledContext is one function per the seven slices'
// worth of identical one-subtest cancelled-context functions this merge
// collapsed. Every method that talks to the pool gets a subtest; the scenario
// per method is distinct, the body shape is not.
func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelled := func() context.Context {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		return ctx
	}

	t.Run("Create", func(t *testing.T) {
		userID := seedUser(t)
		err := New(database.DB{Primary: testPool}).Create(cancelled(), newOrder(userID))
		assert.Error(t, err)
	})

	t.Run("CreateItems", func(t *testing.T) {
		userID := seedUser(t)
		o := seedOrder(t, userID)
		productID := seedProduct(t)

		err := New(database.DB{Primary: testPool}).CreateItems(cancelled(), []domain.Item{
			{
				OrderID:     o.ID,
				ProductID:   productID,
				ProductName: "Widget",
				Price:       money.New(1000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(1000, "USD"),
			},
		})
		assert.Error(t, err)
	})

	t.Run("GetByUserIDAndIdempotencyKey", func(t *testing.T) {
		_, err := New(database.DB{Primary: testPool}).GetByUserIDAndIdempotencyKey(cancelled(), uuid.New(), "key")
		assert.Error(t, err)
	})

	t.Run("UpdateTotals", func(t *testing.T) {
		err := New(database.DB{Primary: testPool}).UpdateTotals(cancelled(), uuid.New(), 0, 0)
		assert.Error(t, err)
	})

	t.Run("GetByID", func(t *testing.T) {
		_, err := New(database.DB{Primary: testPool}).GetByID(cancelled(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("ListByUser", func(t *testing.T) {
		_, err := New(database.DB{Primary: testPool}).ListByUser(cancelled(), uuid.New(), paging.CursorPage{Limit: 10})
		assert.Error(t, err)
	})

	t.Run("ListAdmin", func(t *testing.T) {
		_, _, err := New(database.DB{Primary: testPool}).ListAdmin(cancelled(), order.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10},
		})
		assert.Error(t, err)
	})

	t.Run("ListItemsByOrderID", func(t *testing.T) {
		_, err := New(database.DB{Primary: testPool}).ListItemsByOrderID(cancelled(), uuid.New())
		assert.Error(t, err)
	})

	t.Run("HasDeliveredOrder", func(t *testing.T) {
		_, err := New(database.DB{Primary: testPool}).HasDeliveredOrder(cancelled(), order.DeliveredPurchaseParams{
			UserID:    uuid.New(),
			OrderID:   uuid.New(),
			ProductID: uuid.New(),
		})
		assert.Error(t, err)
	})

	t.Run("GetExpiredOrders", func(t *testing.T) {
		_, err := New(database.DB{Primary: testPool}).GetExpiredOrders(cancelled(), 10)
		assert.Error(t, err)
	})

	t.Run("GetStaleProcessingOrders", func(t *testing.T) {
		_, err := New(database.DB{Primary: testPool}).GetStaleProcessingOrders(cancelled(), 30*time.Minute, 10)
		assert.Error(t, err)
	})

	t.Run("Apply", func(t *testing.T) {
		err := New(database.DB{Primary: testPool}).Apply(cancelled(), uuid.New(), domain.PaidTransition)
		assert.Error(t, err)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		err := New(
			database.DB{Primary: testPool},
		).UpdateStatus(cancelled(), uuid.New(), domain.StatusAwaitingPayment, domain.StatusPaid)
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testutil.SeedUser(t, testPool)
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

// seedOrder is the default fixture: awaiting_payment, 1000 USD.
func seedOrder(t *testing.T, userID uuid.UUID) *domain.Order {
	t.Helper()
	return seedOrderWith(t, userID, domain.StatusAwaitingPayment, money.New(1000, "USD"))
}

// seedOrderWith inserts a minimal order with raw SQL rather than through
// Create, so a fixture never fails because the method under test's sibling is
// broken.
func seedOrderWith(t *testing.T, userID uuid.UUID, status domain.Status, total money.Money) *domain.Order {
	t.Helper()
	o := newOrder(userID)
	o.UserID = userID
	o.Status = status
	o.Subtotal = total
	o.Total = total
	o.Discount = money.New(0, total.Currency)
	// request_hash is left NULL: nothing reading this fixture looks at it, and
	// the NULL-scan path it feeds has its own seeded rows.
	o.RequestHash = ""

	err := testPool.QueryRow(context.Background(),
		`INSERT INTO orders
		   (user_id, idempotency_key, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, $3, $4, 0, $4, $5) RETURNING id, created_at, updated_at`,
		userID, o.IdempotencyKey, status, total.Amount, total.Currency,
	).Scan(&o.ID, &o.CreatedAt, &o.UpdatedAt)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, o.ID) })
	return o
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
