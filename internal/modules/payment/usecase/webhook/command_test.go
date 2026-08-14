package webhook

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// No secret is configured in these tests, so Execute skips signature
// verification and goes straight to the business dispatch it exists to
// prove; TestWebhookHandler_SignatureVerification (webhook/http) covers the
// signature check itself through the real HTTP path.

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success event with already succeeded payment", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _ := newTestCommand(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		payload := marshal(t, map[string]any{
			"event":          "success",
			"transaction_id": "txn_123",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		})

		err := cmd.Execute(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("failed event cancels payment", func(t *testing.T) {
		t.Parallel()

		cmd, repo, orders, _, jobs := newTestCommand(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, paymentID, domain.StatusCancelled,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing}).
			Return(nil)

		repo.EXPECT().ClearPaymentURL(mock.Anything, paymentID).
			Return(nil)

		jobs.EXPECT().CancelPendingByOrderID(mock.Anything, orderID).
			Return(nil)

		// A failed payment also cancels the order and releases its reserved stock.
		orders.EXPECT().CancelUnpaid(mock.Anything, orderID).
			Return(nil)

		payload := marshal(t, map[string]any{
			"event":          "failed",
			"transaction_id": "txn_456",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		})

		err := cmd.Execute(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("expired event cancels payment", func(t *testing.T) {
		t.Parallel()

		cmd, repo, orders, _, jobs := newTestCommand(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusProcessing,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, paymentID, domain.StatusCancelled,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing}).
			Return(nil)

		repo.EXPECT().ClearPaymentURL(mock.Anything, paymentID).
			Return(nil)

		jobs.EXPECT().CancelPendingByOrderID(mock.Anything, orderID).
			Return(nil)

		// An expired payment also cancels the order and releases its reserved stock.
		orders.EXPECT().CancelUnpaid(mock.Anything, orderID).
			Return(nil)

		payload := marshal(t, map[string]any{
			"event":          "expired",
			"transaction_id": "txn_789",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		})

		err := cmd.Execute(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("unknown payment returns nil", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _ := newTestCommand(t)

		unknownID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, unknownID).
			Return(nil, apperror.ErrNotFound)

		payload := marshal(t, map[string]any{
			"event": "success",
			"metadata": map[string]any{
				"payment_id": unknownID.String(),
			},
		})

		err := cmd.Execute(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("no metadata falls back to gateway txn lookup", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _ := newTestCommand(t)

		paymentID := uuid.New()

		p := &domain.Payment{
			ID:     paymentID,
			Status: domain.StatusRefunded,
		}

		repo.EXPECT().GetByGatewayTxnID(mock.Anything, "txn_fallback").
			Return(p, nil)

		payload := marshal(t, map[string]any{
			"event":          "success",
			"transaction_id": "txn_fallback",
		})

		err := cmd.Execute(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("no metadata and no txn_id returns nil", func(t *testing.T) {
		t.Parallel()

		cmd, _, _, _, _ := newTestCommand(t)

		payload := marshal(t, map[string]any{
			"event": "success",
		})

		err := cmd.Execute(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("success event finalizes payment", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, finalizer, jobs := newTestCommand(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil)

		finalizer.EXPECT().
			FinalizePaymentSuccess(mock.Anything, mock.MatchedBy(func(j domain.Job) bool {
				return j.PaymentID == paymentID && j.OrderID == orderID && j.Action == domain.ActionCharge
			})).
			Return(nil)

		jobs.EXPECT().MarkJobCompletedByPaymentID(mock.Anything, paymentID, domain.ActionCharge).Return(nil)

		payload := marshal(t, map[string]any{
			"event":          "success",
			"transaction_id": "txn_success",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		})

		err := cmd.Execute(ctx, payload, "")

		require.NoError(t, err)
	})
}

func marshal(t *testing.T, v map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func newTestCommand(t *testing.T) (*Command, *MockRepository, *MockOrderUpdater, *MockPaymentFinalizer, *MockJobStore) {
	repo := NewMockRepository(t)
	orders := NewMockOrderUpdater(t)
	finalizer := NewMockPaymentFinalizer(t)
	jobs := NewMockJobStore(t)

	cmd := New(repo, orders, finalizer, jobs, "", testhelper.DiscardLogger())

	return cmd, repo, orders, finalizer, jobs
}
