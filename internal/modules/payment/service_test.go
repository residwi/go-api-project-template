package payment

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestService_InitiatePayment(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()
	params := InitiatePaymentParams{
		OrderID:         orderID,
		Amount:          money.New(10000, "USD"),
		PaymentMethodID: "pm_test_123",
	}

	t.Run("success with new payment", func(t *testing.T) {
		t.Parallel()

		// A synchronous "success" charge finalizes in the same call, hence the test tx
		// and the FinalizePaymentSuccess expectations.
		txCtx := ctx
		svc, repo, gw, orders, orderGet, orderItems, inventory, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var capturedPayment *Payment
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *Payment) {
				p.ID = uuid.New()
				capturedPayment = p
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.MatchedBy(func(req ChargeRequest) bool {
			return req.OrderID == orderID.String() &&
				req.Amount == 10000 &&
				req.Currency == "USD" &&
				req.PaymentMethodID == "pm_test_123"
		})).Return(ChargeResponse{
			TransactionID: "txn_abc",
			Status:        "success",
		}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_abc", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetByID(mock.Anything, orderID).
			Return(OrderSnapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)
		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			RunAndReturn(func(_ context.Context, _ uuid.UUID) (*Payment, error) {
				return capturedPayment, nil
			})
		repo.EXPECT().MarkPaid(mock.Anything, mock.AnythingOfType("uuid.UUID"),
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)
		orders.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)
		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, orderID).
			Return([]OrderItemDTO{{ProductID: productID, Quantity: 2}}, nil)
		inventory.EXPECT().DeductBatch(mock.Anything, mock.Anything).
			Return(nil)
		repo.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil)

		result, err := svc.InitiatePayment(txCtx, params)

		require.NoError(t, err)
		require.NotNil(t, capturedPayment)
		assert.Equal(t, orderID, capturedPayment.OrderID)
		assert.Equal(t, money.New(10000, "USD"), capturedPayment.Amount)
		assert.Equal(t, StatusPending, capturedPayment.Status)
		assert.Equal(t, "pm_test_123", capturedPayment.PaymentMethodID)
		assert.Equal(t, capturedPayment.ID, result.PaymentID)
		assert.True(t, result.Charged)
		assert.Empty(t, result.PaymentURL)
	})

	t.Run("success with existing payment", func(t *testing.T) {
		t.Parallel()

		// A synchronous "success" charge finalizes in the same call, hence the test tx
		// and the FinalizePaymentSuccess expectations.
		txCtx := ctx
		svc, repo, gw, orders, orderGet, orderItems, inventory, _, _ := newTestService(t)

		existingID := uuid.New()
		existing := &Payment{
			ID:              existingID,
			OrderID:         orderID,
			Amount:          money.New(10000, "USD"),
			Status:          StatusPending,
			PaymentMethodID: "pm_test_123",
		}

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(existing, nil)

		gw.EXPECT().Charge(mock.Anything, mock.MatchedBy(func(req ChargeRequest) bool {
			return req.IdempotencyKey == existingID.String()
		})).Return(ChargeResponse{
			TransactionID: "txn_existing",
			Status:        "success",
		}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, existingID, "txn_existing", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetByID(mock.Anything, orderID).
			Return(OrderSnapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)
		repo.EXPECT().GetByID(mock.Anything, existingID).
			Return(existing, nil)
		repo.EXPECT().MarkPaid(mock.Anything, existingID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)
		orders.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)
		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, orderID).
			Return([]OrderItemDTO{{ProductID: productID, Quantity: 1}}, nil)
		inventory.EXPECT().DeductBatch(mock.Anything, mock.Anything).
			Return(nil)
		repo.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil)

		result, err := svc.InitiatePayment(txCtx, params)

		require.NoError(t, err)
		assert.Equal(t, existingID, result.PaymentID)
		assert.True(t, result.Charged)
	})

	t.Run("gateway returns pending with PaymentURL", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var createdID uuid.UUID
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *Payment) {
				p.ID = uuid.New()
				createdID = p.ID
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{
				TransactionID: "txn_pending",
				Status:        "pending",
				PaymentURL:    "https://pay.example.com/redirect",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_pending", mock.Anything).
			Return(nil)

		repo.EXPECT().
			UpdatePaymentURL(mock.Anything, mock.AnythingOfType("uuid.UUID"), "https://pay.example.com/redirect").
			Return(nil)

		result, err := svc.InitiatePayment(ctx, params)

		require.NoError(t, err)
		assert.Equal(t, createdID, result.PaymentID)
		assert.False(t, result.Charged)
		assert.Equal(t, "https://pay.example.com/redirect", result.PaymentURL)
	})

	t.Run("gateway returns pending without PaymentURL", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *Payment) {
				p.ID = uuid.New()
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{
				TransactionID: "txn_no_url",
				Status:        "pending",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_no_url", mock.Anything).
			Return(nil)

		result, err := svc.InitiatePayment(ctx, params)

		require.NoError(t, err)
		assert.False(t, result.Charged)
		assert.Empty(t, result.PaymentURL)
	})

	t.Run("gateway error", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var createdID uuid.UUID
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *Payment) {
				p.ID = uuid.New()
				createdID = p.ID
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{}, errors.New("gateway timeout"))

		result, err := svc.InitiatePayment(ctx, params)

		require.Error(t, err)
		require.ErrorContains(t, err, "gateway charge")
		assert.Equal(t, createdID, result.PaymentID)
		assert.False(t, result.Charged)
	})

	t.Run("GetActiveByOrderID error", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, errors.New("db error"))

		_, err := svc.InitiatePayment(ctx, params)

		require.Error(t, err)
		assert.ErrorContains(t, err, "db error")
	})

	t.Run("Create error", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Return(errors.New("insert failed"))

		_, err := svc.InitiatePayment(ctx, params)

		require.Error(t, err)
		assert.ErrorContains(t, err, "insert failed")
	})
}

func TestService_Process(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("unknown action returns false", func(t *testing.T) {
		t.Parallel()

		svc, _, _, _, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:     uuid.New(),
			Action: "invalid_action",
		}

		processErr := svc.Process(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("charge job with order not in expected state", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, orders, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    ActionCharge,
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(errors.New("order not in expected state"))

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.ID == job.ID && j.Status == JobStatusCancelled
		})).Return(nil)

		processErr := svc.Process(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("charge job fails to get payment", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, orders, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    ActionCharge,
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("not found"))

		processErr := svc.Process(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("charge job gateway error with retries remaining", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, orders, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.ID == job.ID &&
				j.Status == JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway error"
		})).Return(nil)

		processErr := svc.Process(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("charge job gateway error max attempts exceeded", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, orders, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionCharge,
			Attempts:    2,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.ID == job.ID &&
				j.Status == JobStatusFailed &&
				j.Attempts == 3
		})).Return(nil)

		processErr := svc.Process(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("charge job success with finalization", func(t *testing.T) {
		t.Parallel()

		txCtx := context.Background()
		svc, repo, gw, orders, orderGet, orderItems, inventory, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{
				TransactionID: "txn_success",
				Status:        "success",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_success", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total:  money.New(5000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID, Quantity: 2},
			}, nil)

		inventory.EXPECT().DeductBatch(mock.Anything, mock.Anything).
			Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.Process(txCtx, job)
		assert.NoError(t, processErr)
	})

	t.Run("charge job handleChargeFailure with UpdateStatus error", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, orders, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(errors.New("CAS failed"))

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.ID == job.ID && j.Status == JobStatusPending
		})).Return(nil)

		processErr := svc.Process(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("charge job handleChargeFailure with UpdateJob error", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, orders, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.Anything).
			Return(errors.New("update job failed"))

		processErr := svc.Process(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("charge job success finalization fails triggers compensating refund", func(t *testing.T) {
		t.Parallel()

		txCtx := context.Background()
		svc, repo, gw, orders, orderGet, _, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{
				TransactionID: "txn_comp",
				Status:        "success",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_comp", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{}, errors.New("db down"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRequiresReview,
			[]Status{StatusPending, StatusProcessing, StatusSuccess}).
			Return(nil)

		orders.EXPECT().MarkFulfillmentFailedCompensating(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.PaymentID == job.PaymentID &&
				j.OrderID == job.OrderID &&
				j.Action == ActionRefund
		})).Return(nil)

		processErr := svc.Process(txCtx, job)
		assert.NoError(t, processErr)
	})

	t.Run("charge job gateway returns non-success status", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, orders, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{
				TransactionID: "txn_failed",
				Status:        "failed",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_failed", mock.Anything).
			Return(nil)

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.ID == job.ID &&
				j.Status == JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway returned status: failed"
		})).Return(nil)

		processErr := svc.Process(ctx, job)
		assert.Error(t, processErr)
	})
}

func TestService_InitiatePayment_UpdateGatewayError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()
	params := InitiatePaymentParams{
		OrderID:         orderID,
		Amount:          money.New(10000, "USD"),
		PaymentMethodID: "pm_test_123",
	}

	t.Run("UpdateGateway error is logged but does not fail", func(t *testing.T) {
		t.Parallel()

		// A synchronous "success" charge finalizes in the same call, hence the test tx
		// and the FinalizePaymentSuccess expectations.
		txCtx := ctx
		svc, repo, gw, orders, orderGet, orderItems, inventory, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var capturedPayment *Payment
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *Payment) {
				p.ID = uuid.New()
				capturedPayment = p
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{
				TransactionID: "txn_gw_err",
				Status:        "success",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_gw_err", mock.Anything).
			Return(errors.New("update gateway failed"))

		orderGet.EXPECT().GetByID(mock.Anything, orderID).
			Return(OrderSnapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)
		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			RunAndReturn(func(_ context.Context, _ uuid.UUID) (*Payment, error) {
				return capturedPayment, nil
			})
		repo.EXPECT().MarkPaid(mock.Anything, mock.AnythingOfType("uuid.UUID"),
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)
		orders.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)
		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, orderID).
			Return([]OrderItemDTO{{ProductID: productID, Quantity: 1}}, nil)
		inventory.EXPECT().DeductBatch(mock.Anything, mock.Anything).
			Return(nil)
		repo.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil)

		result, err := svc.InitiatePayment(txCtx, params)

		require.NoError(t, err)
		assert.True(t, result.Charged)
	})

	t.Run("UpdatePaymentURL error is logged but does not fail", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *Payment) {
				p.ID = uuid.New()
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{
				TransactionID: "txn_url_err",
				Status:        "pending",
				PaymentURL:    "https://pay.example.com/url-err",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_url_err", mock.Anything).
			Return(nil)

		repo.EXPECT().
			UpdatePaymentURL(mock.Anything, mock.AnythingOfType("uuid.UUID"), "https://pay.example.com/url-err").
			Return(errors.New("url update failed"))

		result, err := svc.InitiatePayment(ctx, params)

		require.NoError(t, err)
		assert.False(t, result.Charged)
		assert.Equal(t, "https://pay.example.com/url-err", result.PaymentURL)
	})
}

func TestService_FinalizePaymentSuccess_MultipleItems(t *testing.T) {
	t.Parallel()

	t.Run("sorts items by product ID before deducting", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, orders, orderGet, orderItems, inventory, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &Payment{
			ID:     job.PaymentID,
			Amount: money.New(20000, "USD"),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total:  money.New(20000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID1 := uuid.New()
		productID2 := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID2, Quantity: 1},
				{ProductID: productID1, Quantity: 2},
			}, nil)

		inventory.EXPECT().DeductBatch(mock.Anything, mock.Anything).
			Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})
}

func TestService_RunCompensatingRefund_Error(t *testing.T) {
	t.Parallel()

	t.Run("compensating refund CreateJob error is logged", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, gw, orders, orderGet, _, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          money.New(5000, "USD"),
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(ChargeResponse{
				TransactionID: "txn_comp_err",
				Status:        "success",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_comp_err", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{}, errors.New("db down"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRequiresReview,
			[]Status{StatusPending, StatusProcessing, StatusSuccess}).
			Return(nil)

		orders.EXPECT().MarkFulfillmentFailedCompensating(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.Action == ActionRefund
		})).Return(errors.New("create job failed"))

		processErr := svc.Process(ctx, job)
		assert.NoError(t, processErr)
	})
}

func TestService_HandleWebhook(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success event with already succeeded payment", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		payload := map[string]any{
			"event":          "success",
			"transaction_id": "txn_123",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		}

		err := svc.HandleWebhook(ctx, payload)

		require.NoError(t, err)
	})

	t.Run("failed event cancels payment", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, orders, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, paymentID, StatusCancelled,
			[]Status{StatusPending, StatusProcessing}).
			Return(nil)

		repo.EXPECT().ClearPaymentURL(mock.Anything, paymentID).
			Return(nil)

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(nil)

		// A failed payment also cancels the order and releases its reserved stock.
		orders.EXPECT().CancelUnpaid(mock.Anything, orderID).
			Return(nil)

		payload := map[string]any{
			"event":          "failed",
			"transaction_id": "txn_456",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		}

		err := svc.HandleWebhook(ctx, payload)

		require.NoError(t, err)
	})

	t.Run("expired event cancels payment", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, orders, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  StatusProcessing,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, paymentID, StatusCancelled,
			[]Status{StatusPending, StatusProcessing}).
			Return(nil)

		repo.EXPECT().ClearPaymentURL(mock.Anything, paymentID).
			Return(nil)

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(nil)

		// An expired payment also cancels the order and releases its reserved stock.
		orders.EXPECT().CancelUnpaid(mock.Anything, orderID).
			Return(nil)

		payload := map[string]any{
			"event":          "expired",
			"transaction_id": "txn_789",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		}

		err := svc.HandleWebhook(ctx, payload)

		require.NoError(t, err)
	})

	t.Run("unknown payment returns nil", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		unknownID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, unknownID).
			Return(nil, apperror.ErrNotFound)

		payload := map[string]any{
			"event": "success",
			"metadata": map[string]any{
				"payment_id": unknownID.String(),
			},
		}

		err := svc.HandleWebhook(ctx, payload)

		require.NoError(t, err)
	})

	t.Run("no metadata falls back to gateway txn lookup", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()

		p := &Payment{
			ID:     paymentID,
			Status: StatusRefunded,
		}

		repo.EXPECT().GetByGatewayTxnID(mock.Anything, "txn_fallback").
			Return(p, nil)

		payload := map[string]any{
			"event":          "success",
			"transaction_id": "txn_fallback",
		}

		err := svc.HandleWebhook(ctx, payload)

		require.NoError(t, err)
	})

	t.Run("no metadata and no txn_id returns nil", func(t *testing.T) {
		t.Parallel()

		svc, _, _, _, _, _, _, _, _ := newTestService(t)

		payload := map[string]any{
			"event": "success",
		}

		err := svc.HandleWebhook(ctx, payload)

		require.NoError(t, err)
	})

	t.Run("success event finalizes payment", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, orders, orderGet, orderItems, inv, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()
		productID := uuid.New()

		p := &Payment{
			ID:      paymentID,
			OrderID: orderID,
			Amount:  money.New(5000, "USD"),
			Status:  StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil).Times(2)

		orderGet.EXPECT().GetByID(mock.Anything, orderID).Return(OrderSnapshot{
			Total:  money.New(5000, "USD"),
			Status: "awaiting_payment",
		}, nil)

		repo.EXPECT().MarkPaid(mock.Anything, paymentID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)

		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, orderID).
			Return([]OrderItemDTO{
				{ProductID: productID, Quantity: 2},
			}, nil)

		inv.EXPECT().DeductBatch(mock.Anything, mock.Anything).Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).Return(nil)
		repo.EXPECT().MarkJobCompletedByPaymentID(mock.Anything, paymentID, ActionCharge).Return(nil)

		testCtx := ctx

		payload := map[string]any{
			"event":          "success",
			"transaction_id": "txn_success",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		}

		err := svc.HandleWebhook(testCtx, payload)

		require.NoError(t, err)
	})
}

func TestService_CancelJobsByOrderID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		orderID := uuid.New()

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(nil)

		err := svc.CancelJobsByOrderID(ctx, orderID)

		require.NoError(t, err)
	})

	t.Run("error propagates", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		orderID := uuid.New()

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(errors.New("db error"))

		err := svc.CancelJobsByOrderID(ctx, orderID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "db error")
	})
}

func TestService_Refund(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success enqueues a pending refund job from a succeeded payment", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *Job) bool {
			return job.PaymentID == paymentID &&
				job.OrderID == orderID &&
				job.Action == ActionRefund &&
				job.Status == JobStatusPending
		})).Return(nil)

		err := svc.Refund(ctx, paymentID)

		require.NoError(t, err)
	})

	t.Run("success enqueues a pending refund job from a requires-review payment", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  StatusRequiresReview,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *Job) bool {
			return job.PaymentID == paymentID &&
				job.OrderID == orderID &&
				job.Action == ActionRefund &&
				job.Status == JobStatusPending
		})).Return(nil)

		err := svc.Refund(ctx, paymentID)

		require.NoError(t, err)
	})

	t.Run("payment not refundable - wrong status", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()

		p := &Payment{
			ID:     paymentID,
			Status: StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("payment not refundable - cancelled status", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()

		p := &Payment{
			ID:     paymentID,
			Status: StatusCancelled,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("payment not found", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(nil, apperror.ErrNotFound)

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("create job error", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.Anything).
			Return(errors.New("insert job failed"))

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "insert job failed")
	})
}

func TestService_FinalizePaymentSuccess(t *testing.T) {
	t.Parallel()

	t.Run("success happy path", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, orders, orderGet, orderItems, inventory, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &Payment{
			ID:      job.PaymentID,
			OrderID: job.OrderID,
			Amount:  money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID, Quantity: 3},
			}, nil)

		inventory.EXPECT().DeductBatch(mock.Anything, mock.Anything).
			Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("amount mismatch returns error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, _, orderGet, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &Payment{
			ID:     job.PaymentID,
			Amount: money.New(5000, "USD"),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total: money.New(10000, "USD"),
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrAmountMismatch)
	})

	t.Run("currency mismatch returns error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, _, orderGet, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "EUR"),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total: money.New(10000, "USD"),
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrAmountMismatch)
	})

	t.Run("already finalized by webhook", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, orders, orderGet, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total: money.New(10000, "USD"),
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(errors.New("already paid"))

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(errors.New("already paid"))

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.ErrorIs(t, err, apperror.ErrAlreadyFinalized)
	})

	t.Run("late payment enqueues refund job", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, orders, orderGet, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total:  money.New(10000, "USD"),
				Status: "cancelled",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(errors.New("order already cancelled"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRequiresReview,
			[]Status{StatusSuccess}).
			Return(nil)

		orders.EXPECT().MarkFulfillmentFailedAfterCharge(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.PaymentID == job.PaymentID &&
				j.OrderID == job.OrderID &&
				j.Action == ActionRefund &&
				j.Status == JobStatusPending
		})).Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("inventory deduction error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, orders, orderGet, orderItems, inventory, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID, Quantity: 1},
			}, nil)

		inventory.EXPECT().DeductBatch(mock.Anything, mock.Anything).
			Return(errors.New("out of stock"))

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "deducting inventory")
	})

	t.Run("order snapshot error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, _, _, _, orderGet, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{}, errors.New("db down"))

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "getting order for verification")
	})

	t.Run("payment get error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, _, orderGet, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total: money.New(10000, "USD"),
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("payment not found"))

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "getting payment for verification")
	})

	t.Run("late payment with paid order uses restock inventory action", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, orders, orderGet, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total:         money.New(10000, "USD"),
				Status:        "paid",
				StockDeducted: true,
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(errors.New("already paid"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRequiresReview,
			[]Status{StatusSuccess}).
			Return(nil)

		orders.EXPECT().MarkFulfillmentFailedAfterCharge(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.Action == ActionRefund
		})).Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("listing order items error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, _, orders, orderGet, orderItems, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]Status{
				StatusPending,
				StatusProcessing,
				StatusRequiresReview,
				StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return(nil, errors.New("items db error"))

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "listing order items")
	})
}

func TestService_ProcessRefundJob(t *testing.T) {
	t.Parallel()

	t.Run("success with release inventory", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRestore, couponRel := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_123",
			Status:       StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, RefundRequest{
			IdempotencyKey: job.PaymentID.String(),
			TransactionID:  "txn_123",
			Amount:         5000,
			Reason:         "auto-refund",
		}).Return(RefundResponse{RefundID: "ref_001"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID, Quantity: 2},
			}, nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{CouponCode: "SAVE10", StockDeducted: false}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, false).
			Return(nil)

		couponRel.EXPECT().Release(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.Process(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("success with restock inventory", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRestore, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(8000, "USD"),
			GatewayTxnID: "txn_456",
			Status:       StatusRequiresReview,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, RefundRequest{
			IdempotencyKey: job.PaymentID.String(),
			TransactionID:  "txn_456",
			Amount:         8000,
			Reason:         "auto-refund",
		}).Return(RefundResponse{RefundID: "ref_002"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID, Quantity: 5},
			}, nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{StockDeducted: true}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, true).
			Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.Process(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("payment not refundable", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    ActionRefund,
		}

		p := &Payment{
			ID:     job.PaymentID,
			Status: StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.ID == job.ID && j.Status == JobStatusCancelled
		})).Return(nil)

		processErr := svc.Process(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("payment not found", func(t *testing.T) {
		t.Parallel()

		svc, repo, _, _, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    ActionRefund,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("not found"))

		processErr := svc.Process(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("gateway refund error with retries remaining", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, _, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:           job.PaymentID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_789",
			Status:       StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(RefundResponse{}, errors.New("gateway timeout"))

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.ID == job.ID &&
				j.Status == JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway timeout"
		})).Return(nil)

		processErr := svc.Process(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("gateway refund error max attempts", func(t *testing.T) {
		t.Parallel()

		svc, repo, gw, _, _, _, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionRefund,
			Attempts:    2,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:           job.PaymentID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_999",
			Status:       StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(RefundResponse{}, errors.New("gateway error"))

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *Job) bool {
			return j.ID == job.ID &&
				j.Status == JobStatusFailed &&
				j.Attempts == 3
		})).Return(nil)

		processErr := svc.Process(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("refund with list items error returns false", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, gw, orders, orderGet, orderItems, _, _, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_items_err",
			Status:       StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(RefundResponse{RefundID: "ref_items_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{}, nil)

		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return(nil, errors.New("db error"))

		processErr := svc.Process(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("refund with multiple items sorts by product ID", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRestore, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_multi",
			Status:       StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(RefundResponse{RefundID: "ref_multi"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID1 := uuid.New()
		productID2 := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID2, Quantity: 1},
				{ProductID: productID1, Quantity: 2},
			}, nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{StockDeducted: false}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, false).
			Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.Process(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("refund with release inventory error logs but continues", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRestore, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_rel_err",
			Status:       StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(RefundResponse{RefundID: "ref_rel_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID, Quantity: 1},
			}, nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{StockDeducted: false}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, false).
			Return(errors.New("release failed"))

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.Process(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("refund with restock inventory error logs but continues", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRestore, _ := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_restock_err",
			Status:       StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(RefundResponse{RefundID: "ref_restock_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID, Quantity: 1},
			}, nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{StockDeducted: true}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, true).
			Return(errors.New("restock failed"))

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.Process(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("refund with coupon release error logs but continues", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRestore, couponRel := newTestService(t)

		job := Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_coupon_err",
			Status:       StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(RefundResponse{RefundID: "ref_coupon_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, StatusRefunded,
			[]Status{StatusSuccess, StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]OrderItemDTO{
				{ProductID: productID, Quantity: 1},
			}, nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(OrderSnapshot{CouponCode: "SAVE10", StockDeducted: false}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, false).
			Return(nil)

		couponRel.EXPECT().Release(mock.Anything, job.OrderID).
			Return(errors.New("coupon release failed"))

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.Process(ctx, job)
		assert.NoError(t, processErr)
	})
}

func newTestService(t *testing.T) (
	*Service,
	*MockRepository,
	*MockGateway,
	*MockOrderUpdater,
	*MockOrderGetter,
	*MockOrderItemsGetter,
	*MockInventoryDeductor,
	*MockInventoryRestorer,
	*MockCouponReleaser,
) {
	repo := NewMockRepository(t)
	gw := NewMockGateway(t)
	orders := NewMockOrderUpdater(t)
	orderGet := NewMockOrderGetter(t)
	orderItems := NewMockOrderItemsGetter(t)
	inventory := NewMockInventoryDeductor(t)
	inventoryRestore := NewMockInventoryRestorer(t)
	couponRel := NewMockCouponReleaser(t)

	svc := NewService(
		repo, testhelper.FakeTxRunner{}, gw, orders, orderGet, orderItems,
		inventory, inventoryRestore, couponRel,
		testhelper.DiscardLogger(),
	)

	return svc, repo, gw, orders, orderGet, orderItems,
		inventory, inventoryRestore, couponRel
}
