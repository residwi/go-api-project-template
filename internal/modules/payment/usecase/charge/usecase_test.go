package charge

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommand_Charge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()
	params := contract.ChargeRequest{
		OrderID:         orderID,
		Amount:          money.New(10000, "USD"),
		PaymentMethodID: "pm_test_123",
	}

	t.Run("success with new payment", func(t *testing.T) {
		t.Parallel()

		// A synchronous "success" charge finalizes in the same call, hence the
		// full finalize chain below.
		cmd, repo, gw, orders, orderGet, orderItems, inventory, jobs := newTestCommand(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var capturedPayment *domain.Payment
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
				capturedPayment = p
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.MatchedBy(func(req gateway.ChargeRequest) bool {
			return req.OrderID == orderID.String() &&
				req.Amount == 10000 &&
				req.Currency == "USD" &&
				req.PaymentMethodID == "pm_test_123"
		})).Return(gateway.ChargeResponse{
			TransactionID: "txn_abc",
			Status:        "success",
		}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_abc", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, orderID).
			Return(ordercontract.Order{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)
		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			RunAndReturn(func(_ context.Context, _ uuid.UUID) (*domain.Payment, error) {
				return capturedPayment, nil
			})
		repo.EXPECT().MarkPaid(mock.Anything, mock.AnythingOfType("uuid.UUID"),
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)
		orders.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)
		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, orderID).
			Return(map[uuid.UUID]int{productID: 2}, nil)
		inventory.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)
		jobs.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil)

		result, err := cmd.Charge(ctx, params)

		require.NoError(t, err)
		require.NotNil(t, capturedPayment)
		assert.Equal(t, orderID, capturedPayment.OrderID)
		assert.Equal(t, money.New(10000, "USD"), capturedPayment.Amount)
		assert.Equal(t, domain.StatusPending, capturedPayment.Status)
		assert.Equal(t, "pm_test_123", capturedPayment.PaymentMethodID)
		assert.Equal(t, capturedPayment.ID, result.PaymentID)
		assert.True(t, result.Charged)
		assert.Empty(t, result.PaymentURL)
	})

	t.Run("success with existing payment", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, orders, orderGet, orderItems, inventory, jobs := newTestCommand(t)

		existingID := uuid.New()
		existing := &domain.Payment{
			ID:              existingID,
			OrderID:         orderID,
			Amount:          money.New(10000, "USD"),
			Status:          domain.StatusPending,
			PaymentMethodID: "pm_test_123",
		}

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(existing, nil)

		gw.EXPECT().Charge(mock.Anything, mock.MatchedBy(func(req gateway.ChargeRequest) bool {
			return req.IdempotencyKey == existingID.String()
		})).Return(gateway.ChargeResponse{
			TransactionID: "txn_existing",
			Status:        "success",
		}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, existingID, "txn_existing", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, orderID).
			Return(ordercontract.Order{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)
		repo.EXPECT().GetByID(mock.Anything, existingID).
			Return(existing, nil)
		repo.EXPECT().MarkPaid(mock.Anything, existingID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)
		orders.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)
		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, orderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)
		inventory.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)
		jobs.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil)

		result, err := cmd.Charge(ctx, params)

		require.NoError(t, err)
		assert.Equal(t, existingID, result.PaymentID)
		assert.True(t, result.Charged)
	})

	t.Run("gateway returns pending with PaymentURL", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, _, _, _, _, _ := newTestCommand(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var createdID uuid.UUID
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
				createdID = p.ID
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_pending",
				Status:        "pending",
				PaymentURL:    "https://pay.example.com/redirect",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_pending", mock.Anything).
			Return(nil)

		repo.EXPECT().
			UpdatePaymentURL(mock.Anything, mock.AnythingOfType("uuid.UUID"), "https://pay.example.com/redirect").
			Return(nil)

		result, err := cmd.Charge(ctx, params)

		require.NoError(t, err)
		assert.Equal(t, createdID, result.PaymentID)
		assert.False(t, result.Charged)
		assert.Equal(t, "https://pay.example.com/redirect", result.PaymentURL)
	})

	t.Run("gateway returns pending without PaymentURL", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, _, _, _, _, _ := newTestCommand(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_no_url",
				Status:        "pending",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_no_url", mock.Anything).
			Return(nil)

		result, err := cmd.Charge(ctx, params)

		require.NoError(t, err)
		assert.False(t, result.Charged)
		assert.Empty(t, result.PaymentURL)
	})

	t.Run("gateway error", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, _, _, _, _, _ := newTestCommand(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var createdID uuid.UUID
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
				createdID = p.ID
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway timeout"))

		result, err := cmd.Charge(ctx, params)

		require.Error(t, err)
		require.ErrorContains(t, err, "gateway charge")
		assert.Equal(t, createdID, result.PaymentID)
		assert.False(t, result.Charged)
	})

	t.Run("GetActiveByOrderID error", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, _ := newTestCommand(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, errors.New("db error"))

		_, err := cmd.Charge(ctx, params)

		require.Error(t, err)
		assert.ErrorContains(t, err, "db error")
	})

	t.Run("Create error", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, _ := newTestCommand(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Return(errors.New("insert failed"))

		_, err := cmd.Charge(ctx, params)

		require.Error(t, err)
		assert.ErrorContains(t, err, "insert failed")
	})
}

func TestCommand_Charge_UpdateGatewayError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()
	params := contract.ChargeRequest{
		OrderID:         orderID,
		Amount:          money.New(10000, "USD"),
		PaymentMethodID: "pm_test_123",
	}

	t.Run("UpdateGateway error is logged but does not fail", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, orders, orderGet, orderItems, inventory, jobs := newTestCommand(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var capturedPayment *domain.Payment
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
				capturedPayment = p
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_gw_err",
				Status:        "success",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_gw_err", mock.Anything).
			Return(errors.New("update gateway failed"))

		orderGet.EXPECT().GetSnapshot(mock.Anything, orderID).
			Return(ordercontract.Order{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)
		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			RunAndReturn(func(_ context.Context, _ uuid.UUID) (*domain.Payment, error) {
				return capturedPayment, nil
			})
		repo.EXPECT().MarkPaid(mock.Anything, mock.AnythingOfType("uuid.UUID"),
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)
		orders.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)
		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, orderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)
		inventory.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)
		jobs.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil)

		result, err := cmd.Charge(ctx, params)

		require.NoError(t, err)
		assert.True(t, result.Charged)
	})

	t.Run("UpdatePaymentURL error is logged but does not fail", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, _, _, _, _, _ := newTestCommand(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_url_err",
				Status:        "pending",
				PaymentURL:    "https://pay.example.com/url-err",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_url_err", mock.Anything).
			Return(nil)

		repo.EXPECT().
			UpdatePaymentURL(mock.Anything, mock.AnythingOfType("uuid.UUID"), "https://pay.example.com/url-err").
			Return(errors.New("url update failed"))

		result, err := cmd.Charge(ctx, params)

		require.NoError(t, err)
		assert.False(t, result.Charged)
		assert.Equal(t, "https://pay.example.com/url-err", result.PaymentURL)
	})
}

func TestCommand_ProcessCharge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("order not in expected state", func(t *testing.T) {
		t.Parallel()

		cmd, _, _, orders, _, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(errors.New("order not in expected state"))

		jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID && j.Status == domain.JobStatusCancelled
		})).Return(nil)

		processErr := cmd.ProcessCharge(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("fails to get payment", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, orders, _, _, _, _ := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		orders.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("not found"))

		processErr := cmd.ProcessCharge(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("gateway error with retries remaining", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, orders, _, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
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
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway error"
		})).Return(nil)

		processErr := cmd.ProcessCharge(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("gateway error max attempts exceeded", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, orders, _, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Attempts:    2,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
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
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusFailed &&
				j.Attempts == 3
		})).Return(nil)

		processErr := cmd.ProcessCharge(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("success with finalization", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, orders, orderGet, orderItems, inventory, jobs := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
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
			Return(gateway.ChargeResponse{
				TransactionID: "txn_success",
				Status:        "success",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_success", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total:  money.New(5000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 2}, nil)

		inventory.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := cmd.ProcessCharge(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("handleChargeFailure with UpdateStatus error", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, orders, _, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
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
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(errors.New("CAS failed"))

		jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID && j.Status == domain.JobStatusPending
		})).Return(nil)

		processErr := cmd.ProcessCharge(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("handleChargeFailure with UpdateJob error", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, orders, _, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
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
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		jobs.EXPECT().UpdateJob(mock.Anything, mock.Anything).
			Return(errors.New("update job failed"))

		processErr := cmd.ProcessCharge(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("success finalization fails triggers compensating refund", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, orders, orderGet, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
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
			Return(gateway.ChargeResponse{
				TransactionID: "txn_comp",
				Status:        "success",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_comp", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{}, errors.New("db down"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing, domain.StatusSuccess}).
			Return(nil)

		orders.EXPECT().MarkFulfillmentFailedCompensating(mock.Anything, job.OrderID).
			Return(nil)

		jobs.EXPECT().EnqueueRefund(mock.Anything, job.PaymentID, job.OrderID).
			Return(nil)

		processErr := cmd.ProcessCharge(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("gateway returns non-success status", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, orders, _, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
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
			Return(gateway.ChargeResponse{
				TransactionID: "txn_failed",
				Status:        "failed",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_failed", mock.Anything).
			Return(nil)

		orders.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway returned status: failed"
		})).Return(nil)

		processErr := cmd.ProcessCharge(ctx, job)
		assert.Error(t, processErr)
	})
}

func TestCommand_RunCompensatingRefund_Error(t *testing.T) {
	t.Parallel()

	t.Run("compensating refund EnqueueRefund error is logged", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, gw, orders, orderGet, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
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
			Return(gateway.ChargeResponse{
				TransactionID: "txn_comp_err",
				Status:        "success",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_comp_err", mock.Anything).
			Return(nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{}, errors.New("db down"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing, domain.StatusSuccess}).
			Return(nil)

		orders.EXPECT().MarkFulfillmentFailedCompensating(mock.Anything, job.OrderID).
			Return(nil)

		jobs.EXPECT().EnqueueRefund(mock.Anything, job.PaymentID, job.OrderID).
			Return(errors.New("create job failed"))

		processErr := cmd.ProcessCharge(ctx, job)
		assert.NoError(t, processErr)
	})
}

func TestCommand_FinalizePaymentSuccess(t *testing.T) {
	t.Parallel()

	t.Run("success happy path", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, orders, orderGet, orderItems, inventory, jobs := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:      job.PaymentID,
			OrderID: job.OrderID,
			Amount:  money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 3}, nil)

		inventory.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("amount mismatch returns error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, _, orderGet, _, _, _ := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(5000, "USD"),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total: money.New(10000, "USD"),
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrAmountMismatch)
	})

	t.Run("currency mismatch returns error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, _, orderGet, _, _, _ := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "EUR"),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total: money.New(10000, "USD"),
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrAmountMismatch)
	})

	t.Run("already finalized by webhook", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, orders, orderGet, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total: money.New(10000, "USD"),
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(errors.New("already paid"))

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(errors.New("already paid"))

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.ErrorIs(t, err, apperror.ErrAlreadyFinalized)
	})

	t.Run("late payment enqueues refund job", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, orders, orderGet, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total:  money.New(10000, "USD"),
				Status: "cancelled",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(errors.New("order already cancelled"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusSuccess}).
			Return(nil)

		orders.EXPECT().MarkFulfillmentFailedAfterCharge(mock.Anything, job.OrderID).
			Return(nil)

		jobs.EXPECT().EnqueueRefund(mock.Anything, job.PaymentID, job.OrderID).
			Return(nil)

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("inventory deduction error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, orders, orderGet, orderItems, inventory, _ := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)

		inventory.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(errors.New("out of stock"))

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "deducting inventory")
	})

	t.Run("order snapshot error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, _, _, _, orderGet, _, _, _ := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{}, errors.New("db down"))

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "getting order for verification")
	})

	t.Run("payment get error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, _, orderGet, _, _, _ := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total: money.New(10000, "USD"),
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("payment not found"))

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "getting payment for verification")
	})

	t.Run("late payment with paid order uses restock inventory action", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, orders, orderGet, _, _, jobs := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total:         money.New(10000, "USD"),
				Status:        "paid",
				StockDeducted: true,
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(errors.New("already paid"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusSuccess}).
			Return(nil)

		orders.EXPECT().MarkFulfillmentFailedAfterCharge(mock.Anything, job.OrderID).
			Return(nil)

		jobs.EXPECT().EnqueueRefund(mock.Anything, job.PaymentID, job.OrderID).
			Return(nil)

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("listing order items error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, orders, orderGet, orderItems, _, _ := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(nil, errors.New("items db error"))

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "listing order items")
	})
}

func TestCommand_FinalizePaymentSuccess_MultipleItems(t *testing.T) {
	t.Parallel()

	t.Run("deducts every product the map holds", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, _, orders, orderGet, orderItems, inventory, jobs := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(20000, "USD"),
		}

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{
				Total:  money.New(20000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		orders.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID1 := uuid.New()
		productID2 := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID2: 1, productID1: 2}, nil)

		inventory.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := cmd.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})
}

func newTestCommand(t *testing.T) (
	*UseCase,
	*MockRepository,
	*MockGateway,
	*MockOrderUpdater,
	*MockOrderGetter,
	*MockOrderItemsGetter,
	*MockInventoryDeductor,
	*MockJobStore,
) {
	repo := NewMockRepository(t)
	gw := NewMockGateway(t)
	orders := NewMockOrderUpdater(t)
	orderGet := NewMockOrderGetter(t)
	orderItems := NewMockOrderItemsGetter(t)
	inventory := NewMockInventoryDeductor(t)
	jobs := NewMockJobStore(t)

	cmd := New(
		repo,
		testhelper.FakeTxRunner{},
		gw,
		orders,
		orderGet,
		orderItems,
		inventory,
		jobs,
		testhelper.DiscardLogger(),
	)

	return cmd, repo, gw, orders, orderGet, orderItems, inventory, jobs
}
