package payment

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway"
	gatewaymidtrans "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway/midtrans"
	gatewaymock "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway/mock"
	gatewaystripe "github.com/residwi/go-api-project-template/internal/modules/payment/adapter/gateway/stripe"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

// TestNewGateway pins each Config.Gateway string to the concrete
// implementation it must build: a typo in either case string here or in
// service.go's switch fails this test, where LoadConfig's own validation
// (config_test.go) cannot see it -- LoadConfig only checks the string is one
// of the three, not that this switch still routes each one correctly.
func TestNewGateway(t *testing.T) {
	t.Parallel()

	t.Run("stripe", func(t *testing.T) {
		t.Parallel()

		gw := newGateway(Config{Gateway: gatewayStripe, GatewayTimeout: time.Second})

		assert.IsType(t, &gatewaystripe.Gateway{}, gw)
	})

	t.Run("midtrans", func(t *testing.T) {
		t.Parallel()

		gw := newGateway(Config{Gateway: gatewayMidtrans, GatewayTimeout: time.Second})

		assert.IsType(t, &gatewaymidtrans.Gateway{}, gw)
	})

	t.Run("mock", func(t *testing.T) {
		t.Parallel()

		gw := newGateway(Config{Gateway: gatewayMock, GatewayTimeout: time.Second})

		assert.IsType(t, &gatewaymock.Gateway{}, gw)
	})
}

func TestService_Charge(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()
	req := ChargeRequest{
		OrderID:         orderID,
		Amount:          money.New(10000, "USD"),
		PaymentMethodID: "pm_test_123",
	}

	t.Run("success with new payment", func(t *testing.T) {
		t.Parallel()

		// A synchronous "success" charge finalizes in the same call, hence the
		// full finalize chain below.
		svc, d := newTestService(t)

		d.repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var capturedPayment *domain.Payment
		d.repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
				capturedPayment = p
			}).
			Return(nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.MatchedBy(func(r gateway.ChargeRequest) bool {
			return r.OrderID == orderID.String() &&
				r.Amount == 10000 &&
				r.Currency == "USD" &&
				r.PaymentMethodID == "pm_test_123"
		})).Return(gateway.ChargeResponse{
			TransactionID: "txn_abc",
			Status:        "success",
		}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_abc", mock.Anything).
			Return(nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)
		d.repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			RunAndReturn(func(_ context.Context, _ uuid.UUID) (*domain.Payment, error) {
				return capturedPayment, nil
			})
		d.repo.EXPECT().MarkPaid(mock.Anything, mock.AnythingOfType("uuid.UUID"),
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)
		d.orderTransition.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)
		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, orderID).
			Return(map[uuid.UUID]int{productID: 2}, nil)
		d.inventoryDeduct.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)
		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil)

		result, err := svc.Charge(ctx, req)

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

		svc, d := newTestService(t)

		existingID := uuid.New()
		existing := &domain.Payment{
			ID:              existingID,
			OrderID:         orderID,
			Amount:          money.New(10000, "USD"),
			Status:          domain.StatusPending,
			PaymentMethodID: "pm_test_123",
		}

		d.repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(existing, nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.MatchedBy(func(r gateway.ChargeRequest) bool {
			return r.IdempotencyKey == existingID.String()
		})).Return(gateway.ChargeResponse{
			TransactionID: "txn_existing",
			Status:        "success",
		}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, existingID, "txn_existing", mock.Anything).
			Return(nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)
		d.repo.EXPECT().GetByID(mock.Anything, existingID).
			Return(existing, nil)
		d.repo.EXPECT().MarkPaid(mock.Anything, existingID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)
		d.orderTransition.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)
		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, orderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)
		d.inventoryDeduct.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)
		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil)

		result, err := svc.Charge(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, existingID, result.PaymentID)
		assert.True(t, result.Charged)
	})

	t.Run("gateway returns pending with PaymentURL", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		d.repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var createdID uuid.UUID
		d.repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
				createdID = p.ID
			}).
			Return(nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_pending",
				Status:        "pending",
				PaymentURL:    "https://pay.example.com/redirect",
			}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_pending", mock.Anything).
			Return(nil)

		d.repo.EXPECT().
			UpdatePaymentURL(mock.Anything, mock.AnythingOfType("uuid.UUID"), "https://pay.example.com/redirect").
			Return(nil)

		result, err := svc.Charge(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, createdID, result.PaymentID)
		assert.False(t, result.Charged)
		assert.Equal(t, "https://pay.example.com/redirect", result.PaymentURL)
	})

	t.Run("gateway returns pending without PaymentURL", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		d.repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		d.repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
			}).
			Return(nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_no_url",
				Status:        "pending",
			}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_no_url", mock.Anything).
			Return(nil)

		result, err := svc.Charge(ctx, req)

		require.NoError(t, err)
		assert.False(t, result.Charged)
		assert.Empty(t, result.PaymentURL)
	})

	t.Run("gateway error", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		d.repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var createdID uuid.UUID
		d.repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
				createdID = p.ID
			}).
			Return(nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway timeout"))

		result, err := svc.Charge(ctx, req)

		require.Error(t, err)
		require.ErrorContains(t, err, "gateway charge")
		assert.Equal(t, createdID, result.PaymentID)
		assert.False(t, result.Charged)
	})

	t.Run("GetActiveByOrderID error", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		d.repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, errors.New("db error"))

		_, err := svc.Charge(ctx, req)

		require.Error(t, err)
		assert.ErrorContains(t, err, "db error")
	})

	t.Run("Create error", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		d.repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		d.repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Return(errors.New("insert failed"))

		_, err := svc.Charge(ctx, req)

		require.Error(t, err)
		assert.ErrorContains(t, err, "insert failed")
	})
}

func TestService_Charge_UpdateGatewayError(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	orderID := uuid.New()
	req := ChargeRequest{
		OrderID:         orderID,
		Amount:          money.New(10000, "USD"),
		PaymentMethodID: "pm_test_123",
	}

	t.Run("UpdateGateway error is logged but does not fail", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		d.repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		var capturedPayment *domain.Payment
		d.repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
				capturedPayment = p
			}).
			Return(nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_gw_err",
				Status:        "success",
			}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_gw_err", mock.Anything).
			Return(errors.New("update gateway failed"))

		d.orderRead.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)
		d.repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			RunAndReturn(func(_ context.Context, _ uuid.UUID) (*domain.Payment, error) {
				return capturedPayment, nil
			})
		d.repo.EXPECT().MarkPaid(mock.Anything, mock.AnythingOfType("uuid.UUID"),
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)
		d.orderTransition.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)
		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, orderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)
		d.inventoryDeduct.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)
		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil)

		result, err := svc.Charge(ctx, req)

		require.NoError(t, err)
		assert.True(t, result.Charged)
	})

	t.Run("UpdatePaymentURL error is logged but does not fail", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		d.repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, apperror.ErrNotFound)

		d.repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.Payment")).
			Run(func(_ context.Context, p *domain.Payment) {
				p.ID = uuid.New()
			}).
			Return(nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_url_err",
				Status:        "pending",
				PaymentURL:    "https://pay.example.com/url-err",
			}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_url_err", mock.Anything).
			Return(nil)

		d.repo.EXPECT().
			UpdatePaymentURL(mock.Anything, mock.AnythingOfType("uuid.UUID"), "https://pay.example.com/url-err").
			Return(errors.New("url update failed"))

		result, err := svc.Charge(ctx, req)

		require.NoError(t, err)
		assert.False(t, result.Charged)
		assert.Equal(t, "https://pay.example.com/url-err", result.PaymentURL)
	})
}

func TestService_RunChargeJob(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("order not in expected state", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(errors.New("order not in expected state"))

		d.jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID && j.Status == domain.JobStatusCancelled
		})).Return(nil)

		processErr := svc.RunChargeJob(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("fails to get payment", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("not found"))

		processErr := svc.RunChargeJob(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("gateway error with retries remaining", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

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

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		d.orderTransition.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		d.jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway error"
		})).Return(nil)

		processErr := svc.RunChargeJob(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("gateway error max attempts exceeded", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

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

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		d.orderTransition.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		d.jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusFailed &&
				j.Attempts == 3
		})).Return(nil)

		processErr := svc.RunChargeJob(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("success with finalization", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

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

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_success",
				Status:        "success",
			}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_success", mock.Anything).
			Return(nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total:  money.New(5000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		d.orderTransition.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 2}, nil)

		d.inventoryDeduct.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.RunChargeJob(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("handleChargeFailure with UpdateStatus error", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

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

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		d.orderTransition.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(errors.New("CAS failed"))

		d.jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID && j.Status == domain.JobStatusPending
		})).Return(nil)

		processErr := svc.RunChargeJob(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("handleChargeFailure with UpdateJob error", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

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

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		d.orderTransition.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		d.jobs.EXPECT().UpdateJob(mock.Anything, mock.Anything).
			Return(errors.New("update job failed"))

		processErr := svc.RunChargeJob(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("success finalization fails triggers compensating refund", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

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

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_comp",
				Status:        "success",
			}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_comp", mock.Anything).
			Return(nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{}, errors.New("db down"))

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing, domain.StatusSuccess}).
			Return(nil)

		d.orderTransition.EXPECT().MarkFulfillmentFailedCompensating(mock.Anything, job.OrderID).
			Return(nil)

		d.jobs.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(refundJobFor(job.PaymentID, job.OrderID))).
			Return(nil)

		processErr := svc.RunChargeJob(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("gateway returns non-success status", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

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

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_failed",
				Status:        "failed",
			}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_failed", mock.Anything).
			Return(nil)

		d.orderTransition.EXPECT().MarkAwaitingPayment(mock.Anything, job.OrderID).
			Return(nil)

		d.jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway returned status: failed"
		})).Return(nil)

		processErr := svc.RunChargeJob(ctx, job)
		assert.Error(t, processErr)
	})
}

func TestService_CompensateRefund_Error(t *testing.T) {
	t.Parallel()

	t.Run("compensating refund CreateJob error is logged", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

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

		d.orderTransition.EXPECT().MarkPaymentProcessing(mock.Anything, job.OrderID).
			Return(nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_comp_err",
				Status:        "success",
			}, nil)

		d.repo.EXPECT().UpdateGateway(mock.Anything, p.ID, "txn_comp_err", mock.Anything).
			Return(nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{}, errors.New("db down"))

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing, domain.StatusSuccess}).
			Return(nil)

		d.orderTransition.EXPECT().MarkFulfillmentFailedCompensating(mock.Anything, job.OrderID).
			Return(nil)

		d.jobs.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(refundJobFor(job.PaymentID, job.OrderID))).
			Return(errors.New("create job failed"))

		processErr := svc.RunChargeJob(ctx, job)
		assert.NoError(t, processErr)
	})
}

func TestService_FinalizeSuccess(t *testing.T) {
	t.Parallel()

	t.Run("success happy path", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

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

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		d.orderTransition.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 3}, nil)

		d.inventoryDeduct.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizeSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("amount mismatch returns error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(5000, "USD"),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total: money.New(10000, "USD"),
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		err := svc.FinalizeSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrAmountMismatch)
	})

	t.Run("currency mismatch returns error", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "EUR"),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total: money.New(10000, "USD"),
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		err := svc.FinalizeSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrAmountMismatch)
	})

	t.Run("already finalized by webhook", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total: money.New(10000, "USD"),
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(errors.New("already paid"))

		d.orderTransition.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(errors.New("already paid"))

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizeSuccess(ctx, job)
		require.ErrorIs(t, err, apperror.ErrAlreadyFinalized)
	})

	t.Run("late payment enqueues refund job", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total:  money.New(10000, "USD"),
				Status: "cancelled",
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		d.orderTransition.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(errors.New("order already cancelled"))

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusSuccess}).
			Return(nil)

		d.orderTransition.EXPECT().MarkFulfillmentFailedAfterCharge(mock.Anything, job.OrderID).
			Return(nil)

		d.jobs.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(refundJobFor(job.PaymentID, job.OrderID))).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizeSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("inventory deduction error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		d.orderTransition.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)

		d.inventoryDeduct.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(errors.New("out of stock"))

		err := svc.FinalizeSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "deducting inventory")
	})

	t.Run("order snapshot error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{}, errors.New("db down"))

		err := svc.FinalizeSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "getting order for verification")
	})

	t.Run("payment get error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total: money.New(10000, "USD"),
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("payment not found"))

		err := svc.FinalizeSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "getting payment for verification")
	})

	t.Run("late payment with paid order uses restock inventory action", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total:         money.New(10000, "USD"),
				Status:        "paid",
				StockDeducted: true,
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		d.orderTransition.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(errors.New("already paid"))

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusSuccess}).
			Return(nil)

		d.orderTransition.EXPECT().MarkFulfillmentFailedAfterCharge(mock.Anything, job.OrderID).
			Return(nil)

		d.jobs.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(refundJobFor(job.PaymentID, job.OrderID))).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizeSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("listing order items error propagates", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(10000, "USD"),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total:  money.New(10000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		d.orderTransition.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(nil, errors.New("items db error"))

		err := svc.FinalizeSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "listing order items")
	})
}

func TestService_FinalizeSuccess_MultipleItems(t *testing.T) {
	t.Parallel()

	t.Run("deducts every product the map holds", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Amount: money.New(20000, "USD"),
		}

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{
				Total:  money.New(20000, "USD"),
				Status: "awaiting_payment",
			}, nil)

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		d.orderTransition.EXPECT().MarkPaid(mock.Anything, job.OrderID).
			Return(nil)

		productID1 := uuid.New()
		productID2 := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID2: 1, productID1: 2}, nil)

		d.inventoryDeduct.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizeSuccess(ctx, job)
		require.NoError(t, err)
	})
}

func TestService_Refund(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success enqueues a pending refund job from a succeeded payment", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		d.jobs.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(refundJobFor(paymentID, orderID))).
			Return(nil)

		err := svc.Refund(ctx, paymentID)

		require.NoError(t, err)
	})

	t.Run("success enqueues a pending refund job from a requires-review payment", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusRequiresReview,
		}

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		d.jobs.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(refundJobFor(paymentID, orderID))).
			Return(nil)

		err := svc.Refund(ctx, paymentID)

		require.NoError(t, err)
	})

	t.Run("payment not refundable - wrong status", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()

		p := &domain.Payment{
			ID:     paymentID,
			Status: domain.StatusPending,
		}

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("payment not refundable - cancelled status", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()

		p := &domain.Payment{
			ID:     paymentID,
			Status: domain.StatusCancelled,
		}

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("payment not found", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(nil, apperror.ErrNotFound)

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("enqueue error", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		d.jobs.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(refundJobFor(paymentID, orderID))).
			Return(errors.New("insert job failed"))

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "insert job failed")
	})
}

func TestService_RunRefundJob(t *testing.T) {
	t.Parallel()

	t.Run("success with release inventory", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_123",
			Status:       domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Refund(mock.Anything, gateway.RefundRequest{
			IdempotencyKey: job.PaymentID.String(),
			TransactionID:  "txn_123",
			Amount:         5000,
			Reason:         "auto-refund",
		}).Return(gateway.RefundResponse{RefundID: "ref_001"}, nil)

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		d.orderTransition.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 2}, nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{CouponCode: "SAVE10", StockDeducted: false}, nil)

		d.inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventory.Reserved).
			Return(nil)

		d.coupon.EXPECT().Release(mock.Anything, job.OrderID).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.RunRefundJob(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("success with restock inventory", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(8000, "USD"),
			GatewayTxnID: "txn_456",
			Status:       domain.StatusRequiresReview,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Refund(mock.Anything, gateway.RefundRequest{
			IdempotencyKey: job.PaymentID.String(),
			TransactionID:  "txn_456",
			Amount:         8000,
			Reason:         "auto-refund",
		}).Return(gateway.RefundResponse{RefundID: "ref_002"}, nil)

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		d.orderTransition.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 5}, nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{StockDeducted: true}, nil)

		d.inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventory.Deducted).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.RunRefundJob(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("payment not refundable", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    domain.ActionRefund,
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Status: domain.StatusPending,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID && j.Status == domain.JobStatusCancelled
		})).Return(nil)

		processErr := svc.RunRefundJob(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("payment not found", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    domain.ActionRefund,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("not found"))

		processErr := svc.RunRefundJob(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("gateway refund error with retries remaining", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_789",
			Status:       domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{}, errors.New("gateway timeout"))

		d.jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway timeout"
		})).Return(nil)

		processErr := svc.RunRefundJob(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("gateway refund error max attempts", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    2,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_999",
			Status:       domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{}, errors.New("gateway error"))

		d.jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusFailed &&
				j.Attempts == 3
		})).Return(nil)

		processErr := svc.RunRefundJob(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("refund with list items error returns false", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_items_err",
			Status:       domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_items_err"}, nil)

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		d.orderTransition.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{}, nil)

		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(nil, errors.New("db error"))

		processErr := svc.RunRefundJob(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("refund restores every product the map holds", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_multi",
			Status:       domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_multi"}, nil)

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		d.orderTransition.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID1 := uuid.New()
		productID2 := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID2: 1, productID1: 2}, nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{StockDeducted: false}, nil)

		d.inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventory.Reserved).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.RunRefundJob(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("refund with release inventory error logs but continues", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_rel_err",
			Status:       domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_rel_err"}, nil)

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		d.orderTransition.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{StockDeducted: false}, nil)

		d.inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventory.Reserved).
			Return(errors.New("release failed"))

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.RunRefundJob(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("refund with restock inventory error logs but continues", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_restock_err",
			Status:       domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_restock_err"}, nil)

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		d.orderTransition.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{StockDeducted: true}, nil)

		d.inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventory.Deducted).
			Return(errors.New("restock failed"))

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.RunRefundJob(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("refund with coupon release error logs but continues", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		svc, d := newTestService(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_coupon_err",
			Status:       domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		d.gateway.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_coupon_err"}, nil)

		d.repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		d.orderTransition.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, job.OrderID).
			Return(order.Snapshot{CouponCode: "SAVE10", StockDeducted: false}, nil)

		d.inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventory.Reserved).
			Return(nil)

		d.coupon.EXPECT().Release(mock.Anything, job.OrderID).
			Return(errors.New("coupon release failed"))

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := svc.RunRefundJob(ctx, job)
		assert.NoError(t, processErr)
	})
}

// TestService_HandleWebhook ports every subtest that used to live on
// webhook.UseCase, plus (see "success event finalizes payment" and "finalize
// failure runs compensating refund") the coverage that used to require
// webhook/http's real-usecase test harness -- once FinalizeSuccess and
// CompensateRefund are Service methods instead of a mocked PaymentFinalizer
// port, the finalize-on-webhook path is provable here directly, with no HTTP
// involved at all.
func TestService_HandleWebhook(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success event with already succeeded payment", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusSuccess,
		}

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		payload := marshal(t, map[string]any{
			"event":          "success",
			"transaction_id": "txn_123",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		})

		err := svc.HandleWebhook(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("failed event cancels payment", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusPending,
		}

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		d.repo.EXPECT().UpdateStatus(mock.Anything, paymentID, domain.StatusCancelled,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing}).
			Return(nil)

		d.repo.EXPECT().ClearPaymentURL(mock.Anything, paymentID).
			Return(nil)

		d.jobs.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(nil)

		// A failed payment also cancels the order and releases its reserved stock.
		d.orderCancel.EXPECT().CancelUnpaid(mock.Anything, orderID).
			Return(nil)

		payload := marshal(t, map[string]any{
			"event":          "failed",
			"transaction_id": "txn_456",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		})

		err := svc.HandleWebhook(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("expired event cancels payment", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusProcessing,
		}

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		d.repo.EXPECT().UpdateStatus(mock.Anything, paymentID, domain.StatusCancelled,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing}).
			Return(nil)

		d.repo.EXPECT().ClearPaymentURL(mock.Anything, paymentID).
			Return(nil)

		d.jobs.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(nil)

		// An expired payment also cancels the order and releases its reserved stock.
		d.orderCancel.EXPECT().CancelUnpaid(mock.Anything, orderID).
			Return(nil)

		payload := marshal(t, map[string]any{
			"event":          "expired",
			"transaction_id": "txn_789",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		})

		err := svc.HandleWebhook(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("unknown payment returns nil", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		unknownID := uuid.New()

		d.repo.EXPECT().GetByID(mock.Anything, unknownID).
			Return(nil, apperror.ErrNotFound)

		payload := marshal(t, map[string]any{
			"event": "success",
			"metadata": map[string]any{
				"payment_id": unknownID.String(),
			},
		})

		err := svc.HandleWebhook(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("no metadata falls back to gateway txn lookup", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()

		p := &domain.Payment{
			ID:     paymentID,
			Status: domain.StatusRefunded,
		}

		d.repo.EXPECT().GetByGatewayTxnID(mock.Anything, "txn_fallback").
			Return(p, nil)

		payload := marshal(t, map[string]any{
			"event":          "success",
			"transaction_id": "txn_fallback",
		})

		err := svc.HandleWebhook(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("no metadata and no txn_id returns nil", func(t *testing.T) {
		t.Parallel()

		svc, _ := newTestService(t)

		payload := marshal(t, map[string]any{
			"event": "success",
		})

		err := svc.HandleWebhook(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("malformed payload is ignored", func(t *testing.T) {
		t.Parallel()

		svc, _ := newTestService(t)

		err := svc.HandleWebhook(ctx, []byte("not json"), "")

		require.NoError(t, err)
	})

	t.Run("success event finalizes payment", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()
		amount := money.New(10000, "USD")

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusPending,
			Amount:  amount,
		}

		// The outer webhook lookup and FinalizeSuccess's own verification read
		// both resolve the same row, so one expectation with no .Once() covers
		// both calls.
		d.repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil)

		d.orderRead.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{Total: amount, Status: "awaiting_payment"}, nil)

		d.repo.EXPECT().MarkPaid(mock.Anything, paymentID,
			[]domain.Status{
				domain.StatusPending,
				domain.StatusProcessing,
				domain.StatusRequiresReview,
				domain.StatusCancelled,
			}).
			Return(nil)

		d.orderTransition.EXPECT().MarkPaid(mock.Anything, orderID).
			Return(nil)

		productID := uuid.New()
		d.orderRead.EXPECT().ListItemQuantities(mock.Anything, orderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)

		d.inventoryDeduct.EXPECT().Deduct(mock.Anything, mock.Anything).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompleted(mock.Anything, mock.Anything).
			Return(nil)

		d.jobs.EXPECT().MarkJobCompletedByPaymentID(mock.Anything, paymentID, domain.ActionCharge).
			Return(nil)

		payload := marshal(t, map[string]any{
			"event":          "success",
			"transaction_id": "txn_success",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		})

		err := svc.HandleWebhook(ctx, payload, "")

		require.NoError(t, err)
	})

	t.Run("finalize failure runs compensating refund", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusPending,
		}

		d.repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil)

		// A Snapshot failure fails FinalizeSuccess with a generic error --
		// not apperror.ErrAlreadyFinalized -- which is exactly what should
		// trigger the compensating refund below.
		d.orderRead.EXPECT().Snapshot(mock.Anything, orderID).
			Return(order.Snapshot{}, errors.New("db down"))

		d.repo.EXPECT().UpdateStatus(mock.Anything, paymentID, domain.StatusRequiresReview,
			[]domain.Status{domain.StatusPending, domain.StatusProcessing, domain.StatusSuccess}).
			Return(nil)

		d.orderTransition.EXPECT().MarkFulfillmentFailedCompensating(mock.Anything, orderID).
			Return(nil)

		d.jobs.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(refundJobFor(paymentID, orderID))).
			Return(nil)

		payload := marshal(t, map[string]any{
			"event": "success",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		})

		err := svc.HandleWebhook(ctx, payload, "")

		require.NoError(t, err)
	})
}

func TestService_HandleWebhook_SignatureVerification(t *testing.T) {
	t.Parallel()

	const secret = "whsec_test"

	t.Run("valid signature is accepted", func(t *testing.T) {
		t.Parallel()

		svc, d := newTestService(t)
		svc.webhookSecret = secret

		// Unknown payment id: HandleWebhook no-ops and returns nil, which is
		// enough to prove the signature check passed and the body reached the
		// rest of the method.
		unknownID := uuid.New()
		d.repo.EXPECT().GetByID(mock.Anything, unknownID).Return(nil, apperror.ErrNotFound)

		body := marshal(t, map[string]any{
			"event":    "success",
			"metadata": map[string]any{"payment_id": unknownID.String()},
		})

		err := svc.HandleWebhook(context.Background(), body, sign(secret, body))

		require.NoError(t, err)
	})

	t.Run("missing signature is rejected", func(t *testing.T) {
		t.Parallel()

		svc, _ := newTestService(t)
		svc.webhookSecret = secret

		body := marshal(t, map[string]any{"event": "success"})

		err := svc.HandleWebhook(context.Background(), body, "")

		require.ErrorIs(t, err, apperror.ErrUnauthorized)
	})

	t.Run("wrong signature is rejected", func(t *testing.T) {
		t.Parallel()

		svc, _ := newTestService(t)
		svc.webhookSecret = secret

		body := marshal(t, map[string]any{"event": "success"})

		err := svc.HandleWebhook(context.Background(), body, "deadbeef")

		require.ErrorIs(t, err, apperror.ErrUnauthorized)
	})
}

type testDeps struct {
	repo             *MockRepository
	gateway          *fakeGateway
	orderTransition  *MockOrderTransition
	orderCancel      *MockOrderCanceller
	orderRead        *MockOrderReader
	inventoryDeduct  *MockInventoryDeductor
	inventoryRestore *MockInventoryRestorer
	coupon           *MockCouponReleaser
	jobs             *MockJobRepository
}

// newTestService builds *Service directly rather than through New, because
// New builds its JobRepository itself from Deps.DB (see jobs.go) and a
// unit test needs a mock there instead of a real pool.
func newTestService(t *testing.T) (*Service, testDeps) {
	d := testDeps{
		repo:             NewMockRepository(t),
		gateway:          newFakeGateway(t),
		orderTransition:  NewMockOrderTransition(t),
		orderCancel:      NewMockOrderCanceller(t),
		orderRead:        NewMockOrderReader(t),
		inventoryDeduct:  NewMockInventoryDeductor(t),
		inventoryRestore: NewMockInventoryRestorer(t),
		coupon:           NewMockCouponReleaser(t),
		jobs:             NewMockJobRepository(t),
	}

	svc := &Service{
		repo:             d.repo,
		tx:               testutil.FakeTxRunner{},
		gateway:          d.gateway,
		jobs:             d.jobs,
		logger:           testutil.DiscardLogger(),
		orderTransition:  d.orderTransition,
		orderCancel:      d.orderCancel,
		orderRead:        d.orderRead,
		inventoryDeduct:  d.inventoryDeduct,
		inventoryRestore: d.inventoryRestore,
		coupon:           d.coupon,
	}

	return svc, d
}

type fakeGateway struct {
	mock.Mock
}

func newFakeGateway(t interface {
	mock.TestingT
	Cleanup(func())
},
) *fakeGateway {
	f := &fakeGateway{}
	f.Mock.Test(t)
	t.Cleanup(func() { f.AssertExpectations(t) })
	return f
}

func (f *fakeGateway) EXPECT() *fakeGatewayExpecter {
	return &fakeGatewayExpecter{mock: &f.Mock}
}

func (f *fakeGateway) Charge(ctx context.Context, req gateway.ChargeRequest) (gateway.ChargeResponse, error) {
	args := f.Called(ctx, req)
	return args.Get(0).(gateway.ChargeResponse), args.Error(1)
}

func (f *fakeGateway) Refund(ctx context.Context, req gateway.RefundRequest) (gateway.RefundResponse, error) {
	args := f.Called(ctx, req)
	return args.Get(0).(gateway.RefundResponse), args.Error(1)
}

type fakeGatewayExpecter struct {
	mock *mock.Mock
}

func (e *fakeGatewayExpecter) Charge(ctx, req any) *mock.Call {
	return e.mock.On("Charge", ctx, req)
}

func (e *fakeGatewayExpecter) Refund(ctx, req any) *mock.Call {
	return e.mock.On("Refund", ctx, req)
}

// refundJobFor builds the mock.MatchedBy predicate every enqueueRefund call
// site is checked against, now that CreateJob replaces the old three-slice
// JobStore.EnqueueRefund port: the mapping it checks -- payment and order id
// carried onto a pending refund job -- is exactly what the dissolved port's
// exact-argument expectation used to check.
func refundJobFor(paymentID, orderID uuid.UUID) func(*domain.Job) bool {
	return func(j *domain.Job) bool {
		return j.PaymentID == paymentID && j.OrderID == orderID && j.Action == domain.ActionRefund &&
			j.Status == domain.JobStatusPending
	}
}

func sign(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func marshal(t *testing.T, v map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
