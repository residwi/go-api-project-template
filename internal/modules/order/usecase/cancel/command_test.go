package cancel

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _ := newTestCommand(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		err := cmd.Execute(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _ := newTestCommand(t)

		otherUserID := uuid.New()
		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: otherUserID,
			Status: domain.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := cmd.Execute(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("payment processing returns ErrOrderCharging", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _ := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusPaymentProcessing,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		err := cmd.Execute(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrOrderCharging)
	})

	t.Run("invalid transition from delivered", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, _, _ := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusDelivered,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(apperror.ErrConflict)

		err := cmd.Execute(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("invalid transition from paid", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, _, _ := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(apperror.ErrConflict)

		err := cmd.Execute(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("invalid transition from shipped", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, _, _ := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusShipped,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(apperror.ErrConflict)

		err := cmd.Execute(ctx, userID, orderID)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("success cancels awaiting_payment order", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, inventory, paymentCancel := newTestCommand(t)

		productA := uuid.New()
		productB := uuid.New()
		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   productA,
				ProductName: "Widget A",
				Price:       money.New(3000, "USD"),
				Quantity:    2,
				Subtotal:    money.New(6000, "USD"),
			},
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   productB,
				ProductName: "Widget B",
				Price:       money.New(4000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(4000, "USD"),
			},
		}, nil)
		inventory.EXPECT().Restore(mock.Anything, map[uuid.UUID]int{
			productA: 2,
			productB: 1,
		}, inventorycontract.Reserved).Return(nil)
		paymentCancel.EXPECT().CancelPendingByOrderID(mock.Anything, orderID).Return(nil)

		err := cmd.Execute(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("success releases coupon on cancel", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, inventory, coupons, paymentCancel := newTestCommandWithCoupons(t)

		couponCode := "SAVE20"
		existingOrder := &domain.Order{
			ID:         orderID,
			UserID:     userID,
			Status:     domain.StatusAwaitingPayment,
			CouponCode: &couponCode,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   uuid.New(),
				ProductName: "Widget",
				Price:       money.New(5000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(5000, "USD"),
			},
		}, nil)
		inventory.EXPECT().Restore(mock.Anything, mock.Anything, inventorycontract.Reserved).Return(nil)
		coupons.EXPECT().Release(mock.Anything, orderID).Return(nil)
		paymentCancel.EXPECT().CancelPendingByOrderID(mock.Anything, orderID).Return(nil)

		err := cmd.Execute(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("success cancels payment jobs best effort", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, _, paymentCancel := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{}, nil)
		paymentCancel.EXPECT().CancelPendingByOrderID(mock.Anything, orderID).Return(errors.New("redis down"))

		err := cmd.Execute(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("inventory release error fails the cancellation", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, inventory, _ := newTestCommand(t)

		productA := uuid.New()
		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   productA,
				ProductName: "Widget",
				Price:       money.New(5000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(5000, "USD"),
			},
		}, nil)
		inventory.EXPECT().
			Restore(mock.Anything, mock.Anything, inventorycontract.Reserved).
			Return(errors.New("inventory error"))
		// The release failure rolls back the cancellation (the tx returns the error),
		// so payment-job cancellation is never reached.

		err := cmd.Execute(ctx, userID, orderID)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "inventory error")
	})

	t.Run("coupon release error is logged but swallowed", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, inventory, coupons, paymentCancel := newTestCommandWithCoupons(t)

		couponCode := "SAVE20"
		existingOrder := &domain.Order{
			ID:         orderID,
			UserID:     userID,
			Status:     domain.StatusAwaitingPayment,
			CouponCode: &couponCode,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   uuid.New(),
				ProductName: "Widget",
				Price:       money.New(5000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(5000, "USD"),
			},
		}, nil)
		inventory.EXPECT().Restore(mock.Anything, mock.Anything, inventorycontract.Reserved).Return(nil)
		coupons.EXPECT().Release(mock.Anything, orderID).Return(errors.New("coupon service down"))
		paymentCancel.EXPECT().CancelPendingByOrderID(mock.Anything, orderID).Return(nil)

		err := cmd.Execute(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("nil paymentCancel skips job cancellation", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		transition := NewMockTransitionApplier(t)
		inventory := NewMockInventoryRestorer(t)

		cmd := New(repo, testhelper.FakeTxRunner{}, transition, inventory, nil, nil, testhelper.DiscardLogger())

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{}, nil)

		err := cmd.Execute(ctx, userID, orderID)

		assert.NoError(t, err)
	})

	t.Run("Apply error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, _, _ := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(errors.New("db error"))

		err := cmd.Execute(ctx, userID, orderID)

		assert.Error(t, err)
	})

	t.Run("ListItemsByOrderID error propagates from transaction", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, _, _ := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return(nil, errors.New("db error"))

		err := cmd.Execute(ctx, userID, orderID)

		assert.Error(t, err)
	})
}

// TestCommand_CancelUnpaid runs the same cancelWithReversal path as Execute,
// minus the ownership check: the payment webhook has no caller to own the
// order against.
func TestCommand_CancelUnpaid(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()

	t.Run("success, no ownership check", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, inventory, paymentCancel := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: uuid.New(),
			Status: domain.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]domain.Item{}, nil)
		paymentCancel.EXPECT().CancelPendingByOrderID(mock.Anything, orderID).Maybe().Return(nil)
		inventory.AssertNotCalled(t, "Restore", mock.Anything, mock.Anything, mock.Anything)

		err := cmd.CancelUnpaid(ctx, orderID)

		assert.NoError(t, err)
	})

	t.Run("rejects an order already paid", func(t *testing.T) {
		t.Parallel()

		cmd, repo, transition, _, _ := newTestCommand(t)

		existingOrder := &domain.Order{ID: orderID, UserID: uuid.New(), Status: domain.StatusPaid}
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)
		transition.EXPECT().Apply(mock.Anything, orderID, domain.CancelledTransition).Return(apperror.ErrConflict)

		err := cmd.CancelUnpaid(ctx, orderID)

		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})
}

func newTestCommand(t *testing.T) (
	*Command,
	*MockRepository,
	*MockTransitionApplier,
	*MockInventoryRestorer,
	*MockPaymentJobCanceller,
) {
	repo := NewMockRepository(t)
	transition := NewMockTransitionApplier(t)
	inventory := NewMockInventoryRestorer(t)
	paymentCancel := NewMockPaymentJobCanceller(t)

	cmd := New(repo, testhelper.FakeTxRunner{}, transition, inventory, nil, paymentCancel, testhelper.DiscardLogger())
	return cmd, repo, transition, inventory, paymentCancel
}

func newTestCommandWithCoupons(t *testing.T) (
	*Command,
	*MockRepository,
	*MockTransitionApplier,
	*MockInventoryRestorer,
	*MockCouponReleaser,
	*MockPaymentJobCanceller,
) {
	repo := NewMockRepository(t)
	transition := NewMockTransitionApplier(t)
	inventory := NewMockInventoryRestorer(t)
	coupons := NewMockCouponReleaser(t)
	paymentCancel := NewMockPaymentJobCanceller(t)

	cmd := New(
		repo,
		testhelper.FakeTxRunner{},
		transition,
		inventory,
		coupons,
		paymentCancel,
		testhelper.DiscardLogger(),
	)
	return cmd, repo, transition, inventory, coupons, paymentCancel
}
