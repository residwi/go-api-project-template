package expire

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	inventorypkg "github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestUseCase_ExpireStale(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("expires each stale order, releasing its reservation and coupon", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionApplier(t)
		inventory := NewMockInventoryRestorer(t)
		coupons := NewMockCouponReleaser(t)
		cmd := New(repo, testhelper.FakeTxRunner{}, transition, inventory, coupons, testhelper.DiscardLogger())

		coupon := "SAVE10"
		expired := domain.Order{ID: uuid.New(), CouponCode: &coupon}
		productID := uuid.New()

		repo.EXPECT().GetExpiredOrders(mock.Anything, mock.Anything).Return([]domain.Order{expired}, nil)
		transition.EXPECT().Apply(mock.Anything, expired.ID, domain.ExpiredTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, expired.ID).
			Return([]domain.Item{{ProductID: productID, Quantity: 2}}, nil)
		inventory.EXPECT().
			Restore(mock.Anything, map[uuid.UUID]int{productID: 2}, inventorypkg.Reserved).
			Return(nil)
		coupons.EXPECT().Release(mock.Anything, expired.ID).Return(nil)

		err := cmd.ExpireStale(ctx)
		require.NoError(t, err)
	})

	t.Run("skips an order another worker already expired", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionApplier(t)
		inventory := NewMockInventoryRestorer(t)
		cmd := New(repo, testhelper.FakeTxRunner{}, transition, inventory, nil, testhelper.DiscardLogger())

		expired := domain.Order{ID: uuid.New()}
		repo.EXPECT().GetExpiredOrders(mock.Anything, mock.Anything).Return([]domain.Order{expired}, nil)
		transition.EXPECT().Apply(mock.Anything, expired.ID, domain.ExpiredTransition).Return(apperror.ErrConflict)

		err := cmd.ExpireStale(ctx)
		require.NoError(t, err)
	})
}
