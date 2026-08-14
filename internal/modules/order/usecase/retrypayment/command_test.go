package retrypayment

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/money"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	userID := uuid.New()
	orderID := uuid.New()
	paymentMethodID := "pm_test_123"

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		cmd, repo, payment := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
			Total:  money.New(5000, "USD"),
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		expectedResult := paymentcontract.ChargeResult{
			PaymentID:  uuid.New(),
			PaymentURL: "https://pay.example.com/checkout",
			Charged:    false,
		}
		payment.EXPECT().InitiatePayment(mock.Anything, paymentcontract.ChargeRequest{
			OrderID:         orderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: paymentMethodID,
		}).Return(expectedResult, nil)

		result, err := cmd.Execute(ctx, userID, orderID, Params{PaymentMethodID: paymentMethodID})

		require.NoError(t, err)
		assert.Equal(t, &expectedResult, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _ := newTestCommand(t)

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		result, err := cmd.Execute(ctx, userID, orderID, Params{PaymentMethodID: paymentMethodID})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _ := newTestCommand(t)

		otherUserID := uuid.New()
		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: otherUserID,
			Status: domain.StatusAwaitingPayment,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := cmd.Execute(ctx, userID, orderID, Params{PaymentMethodID: paymentMethodID})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("not payable when status is paid", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _ := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusPaid,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := cmd.Execute(ctx, userID, orderID, Params{PaymentMethodID: paymentMethodID})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrOrderNotPayable)
	})

	t.Run("not payable when status is cancelled", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _ := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusCancelled,
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		result, err := cmd.Execute(ctx, userID, orderID, Params{PaymentMethodID: paymentMethodID})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, apperror.ErrOrderNotPayable)
	})

	t.Run("payment initiation fails", func(t *testing.T) {
		t.Parallel()

		cmd, repo, payment := newTestCommand(t)

		existingOrder := &domain.Order{
			ID:     orderID,
			UserID: userID,
			Status: domain.StatusAwaitingPayment,
			Total:  money.New(5000, "USD"),
		}

		repo.EXPECT().GetByID(mock.Anything, orderID).Return(existingOrder, nil)

		paymentErr := errors.New("payment gateway error")
		payment.EXPECT().InitiatePayment(mock.Anything, mock.Anything).
			Return(paymentcontract.ChargeResult{}, paymentErr)

		result, err := cmd.Execute(ctx, userID, orderID, Params{PaymentMethodID: paymentMethodID})

		assert.Nil(t, result)
		assert.ErrorIs(t, err, paymentErr)
	})
}

// TestCommandExecuteUsesPaymentContract pins PaymentInitiator to payment's
// published types, so a signature drift on either side fails the mock's
// argument match here instead of silently compiling against the wrong shape.
func TestCommandExecuteUsesPaymentContract(t *testing.T) {
	t.Parallel()

	cmd, repo, payment := newTestCommand(t)

	userID, orderID, paymentID := uuid.New(), uuid.New(), uuid.New()

	repo.EXPECT().GetByID(mock.Anything, orderID).Return(&domain.Order{
		ID:     orderID,
		UserID: userID,
		Status: domain.StatusAwaitingPayment,
		Total:  money.New(7500, "IDR"),
	}, nil)

	payment.EXPECT().InitiatePayment(mock.Anything, paymentcontract.ChargeRequest{
		OrderID:         orderID,
		Amount:          money.New(7500, "IDR"),
		PaymentMethodID: "card",
	}).Return(paymentcontract.ChargeResult{PaymentID: paymentID, Charged: true}, nil)

	got, err := cmd.Execute(context.Background(), userID, orderID, Params{PaymentMethodID: "card"})

	require.NoError(t, err)
	assert.Equal(t, paymentID, got.PaymentID)
}

func newTestCommand(t *testing.T) (*Command, *MockRepository, *MockPaymentInitiator) {
	repo := NewMockRepository(t)
	payment := NewMockPaymentInitiator(t)

	cmd := New(repo, payment)
	return cmd, repo, payment
}
