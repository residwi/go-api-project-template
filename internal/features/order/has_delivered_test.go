package order_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/order"
)

func TestPostgresRepository_HasDeliveredOrder(t *testing.T) {
	repo := order.NewPostgresRepository(testPool)
	ctx := context.Background()

	seedOrderItem := func(t *testing.T, userID, productID uuid.UUID, status order.Status) uuid.UUID {
		t.Helper()
		orderID := uuid.New()
		_, err := testPool.Exec(ctx,
			`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
			 VALUES ($1, $2, $3, 1000, 0, 1000, 'USD')`,
			orderID, userID, string(status),
		)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx,
			`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal)
			 VALUES ($1, $2, 'Product', 1000, 1, 1000)`,
			orderID, productID,
		)
		require.NoError(t, err)
		return orderID
	}

	t.Run("true when the order is delivered and contains the product", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrderItem(t, userID, productID, order.StatusDelivered)

		ok, err := repo.HasDeliveredOrder(ctx, userID, orderID, productID)

		require.NoError(t, err)
		assert.True(t, ok)
	})

	t.Run("false when the matching order is not delivered", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrderItem(t, userID, productID, order.StatusPaid)

		ok, err := repo.HasDeliveredOrder(ctx, userID, orderID, productID)

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("false when the user has no such order", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)

		ok, err := repo.HasDeliveredOrder(ctx, userID, uuid.New(), productID)

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("false when the delivered order belongs to another user", func(t *testing.T) {
		setup(t)
		buyer := seedUser(t)
		other := seedUser(t)
		productID := seedProduct(t)
		orderID := seedOrderItem(t, buyer, productID, order.StatusDelivered)

		ok, err := repo.HasDeliveredOrder(ctx, other, orderID, productID)

		require.NoError(t, err)
		assert.False(t, ok)
	})

	t.Run("false when the orderID is not the order that delivered the product", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		productID := seedProduct(t)
		seedOrderItem(t, userID, productID, order.StatusDelivered)

		ok, err := repo.HasDeliveredOrder(ctx, userID, uuid.New(), productID)

		require.NoError(t, err)
		assert.False(t, ok)
	})
}
