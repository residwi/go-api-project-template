package payment_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	gateway "github.com/residwi/go-api-project-template/internal/platform/payment"
	mocks "github.com/residwi/go-api-project-template/mocks/payment"
)

type noopDBTX struct{}

func (noopDBTX) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}
func (noopDBTX) Query(context.Context, string, ...any) (pgx.Rows, error) { return nil, nil } //nolint:nilnil // test stub
func (noopDBTX) QueryRow(context.Context, string, ...any) pgx.Row        { return nil }

func newTestService(t *testing.T) (
	*payment.Service,
	*mocks.MockRepository,
	*mocks.MockGateway,
	*mocks.MockOrderUpdater,
	*mocks.MockOrderGetter,
	*mocks.MockOrderItemsGetter,
	*mocks.MockInventoryDeductor,
	*mocks.MockInventoryReleaser,
	*mocks.MockInventoryRestocker,
	*mocks.MockCouponReleaser,
) {
	repo := mocks.NewMockRepository(t)
	gw := mocks.NewMockGateway(t)
	orders := mocks.NewMockOrderUpdater(t)
	orderGet := mocks.NewMockOrderGetter(t)
	orderItems := mocks.NewMockOrderItemsGetter(t)
	inventory := mocks.NewMockInventoryDeductor(t)
	inventoryRel := mocks.NewMockInventoryReleaser(t)
	inventoryRestock := mocks.NewMockInventoryRestocker(t)
	couponRel := mocks.NewMockCouponReleaser(t)

	svc := payment.NewService(
		repo, nil, gw, orders, orderGet, orderItems,
		inventory, inventoryRel, inventoryRestock, couponRel,
	)

	return svc, repo, gw, orders, orderGet, orderItems,
		inventory, inventoryRel, inventoryRestock, couponRel
}

func TestService_InitiatePayment(t *testing.T) {
	ctx := context.Background()
	orderID := uuid.New()
	params := payment.InitiatePaymentParams{
		OrderID:         orderID,
		Amount:          10000,
		Currency:        "USD",
		PaymentMethodID: "pm_test_123",
	}

	t.Run("success with new payment", func(t *testing.T) {
		svc, repo, gw, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, core.ErrNotFound)

		var capturedPayment *payment.Payment
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *payment.Payment) {
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

		result, err := svc.InitiatePayment(ctx, params)

		require.NoError(t, err)
		require.NotNil(t, capturedPayment)
		assert.Equal(t, orderID, capturedPayment.OrderID)
		assert.Equal(t, int64(10000), capturedPayment.Amount)
		assert.Equal(t, "USD", capturedPayment.Currency)
		assert.Equal(t, payment.StatusPending, capturedPayment.Status)
		assert.Equal(t, "pm_test_123", capturedPayment.PaymentMethodID)
		assert.Equal(t, capturedPayment.ID, result.PaymentID)
		assert.True(t, result.Charged)
		assert.Empty(t, result.PaymentURL)
	})

	t.Run("success with existing payment", func(t *testing.T) {
		svc, repo, gw, _, _, _, _, _, _, _ := newTestService(t)

		existingID := uuid.New()
		existing := &payment.Payment{
			ID:              existingID,
			OrderID:         orderID,
			Amount:          10000,
			Currency:        "USD",
			Status:          payment.StatusPending,
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

		result, err := svc.InitiatePayment(ctx, params)

		require.NoError(t, err)
		assert.Equal(t, existingID, result.PaymentID)
		assert.True(t, result.Charged)
	})

	t.Run("gateway returns pending with PaymentURL", func(t *testing.T) {
		svc, repo, gw, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, core.ErrNotFound)

		var createdID uuid.UUID
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *payment.Payment) {
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

		repo.EXPECT().UpdatePaymentURL(mock.Anything, mock.AnythingOfType("uuid.UUID"), "https://pay.example.com/redirect").
			Return(nil)

		result, err := svc.InitiatePayment(ctx, params)

		require.NoError(t, err)
		assert.Equal(t, createdID, result.PaymentID)
		assert.False(t, result.Charged)
		assert.Equal(t, "https://pay.example.com/redirect", result.PaymentURL)
	})

	t.Run("gateway returns pending without PaymentURL", func(t *testing.T) {
		svc, repo, gw, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, core.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *payment.Payment) {
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

		result, err := svc.InitiatePayment(ctx, params)

		require.NoError(t, err)
		assert.False(t, result.Charged)
		assert.Empty(t, result.PaymentURL)
	})

	t.Run("gateway error", func(t *testing.T) {
		svc, repo, gw, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, core.ErrNotFound)

		var createdID uuid.UUID
		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *payment.Payment) {
				p.ID = uuid.New()
				createdID = p.ID
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway timeout"))

		result, err := svc.InitiatePayment(ctx, params)

		require.Error(t, err)
		require.ErrorContains(t, err, "gateway charge")
		assert.Equal(t, createdID, result.PaymentID)
		assert.False(t, result.Charged)
	})

	t.Run("GetActiveByOrderID error", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, errors.New("db error"))

		_, err := svc.InitiatePayment(ctx, params)

		require.Error(t, err)
		assert.ErrorContains(t, err, "db error")
	})

	t.Run("Create error", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, core.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Return(errors.New("insert failed"))

		_, err := svc.InitiatePayment(ctx, params)

		require.Error(t, err)
		assert.ErrorContains(t, err, "insert failed")
	})
}

func TestService_ProcessJob(t *testing.T) {
	ctx := context.Background()

	t.Run("unknown action returns false", func(t *testing.T) {
		svc, _, _, _, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:     uuid.New(),
			Action: "invalid_action",
		}

		result := svc.ProcessJob(ctx, job)
		assert.False(t, result)
	})

	t.Run("charge job with order not in expected state", func(t *testing.T) {
		svc, repo, _, orders, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    payment.ActionCharge,
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
			Return(errors.New("order not in expected state"))

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.ID == job.ID && j.Status == payment.JobStatusCancelled
		})).Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.False(t, result)
	})

	t.Run("charge job fails to get payment", func(t *testing.T) {
		svc, repo, _, orders, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    payment.ActionCharge,
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("not found"))

		result := svc.ProcessJob(ctx, job)
		assert.False(t, result)
	})

	t.Run("charge job gateway error with retries remaining", func(t *testing.T) {
		svc, repo, gw, orders, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing"}, "awaiting_payment").
			Return(nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.ID == job.ID &&
				j.Status == payment.JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway error"
		})).Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.False(t, result)
	})

	t.Run("charge job gateway error max attempts exceeded", func(t *testing.T) {
		svc, repo, gw, orders, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionCharge,
			Attempts:    2,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing"}, "awaiting_payment").
			Return(nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.ID == job.ID &&
				j.Status == payment.JobStatusFailed &&
				j.Attempts == 3
		})).Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.False(t, result)
	})

	t.Run("charge job success with finalization", func(t *testing.T) {
		txCtx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, orderGet, orderItems, inventory, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
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

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 5000,
				Currency:    "USD",
				Status:      "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusRequiresReview, payment.StatusCancelled}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment"}, "paid").
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 2},
			}, nil)

		inventory.EXPECT().Deduct(mock.Anything, productID, 2).
			Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		result := svc.ProcessJob(txCtx, job)
		assert.True(t, result)
	})

	t.Run("charge job handleChargeFailure with UpdateStatus error", func(t *testing.T) {
		svc, repo, gw, orders, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing"}, "awaiting_payment").
			Return(errors.New("CAS failed"))

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.ID == job.ID && j.Status == payment.JobStatusPending
		})).Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.False(t, result)
	})

	t.Run("charge job handleChargeFailure with UpdateJob error", func(t *testing.T) {
		svc, repo, gw, orders, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
			Return(nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, errors.New("gateway error"))

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing"}, "awaiting_payment").
			Return(nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.Anything).
			Return(errors.New("update job failed"))

		result := svc.ProcessJob(ctx, job)
		assert.False(t, result)
	})

	t.Run("charge job success finalization fails triggers compensating refund", func(t *testing.T) {
		txCtx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, orderGet, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
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

		// FinalizePaymentSuccess fails at orderGet
		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{}, errors.New("db down"))

		// runCompensatingRefund expectations
		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRequiresReview,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusSuccess}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment", "cancelled", "expired", "paid"},
			"fulfillment_failed").
			Return(nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.PaymentID == job.PaymentID &&
				j.OrderID == job.OrderID &&
				j.Action == payment.ActionRefund &&
				j.InventoryAction == "release"
		})).Return(nil)

		result := svc.ProcessJob(txCtx, job)
		assert.True(t, result)
	})

	t.Run("charge job gateway returns non-success status", func(t *testing.T) {
		svc, repo, gw, orders, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
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

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing"}, "awaiting_payment").
			Return(nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.ID == job.ID &&
				j.Status == payment.JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway returned status: failed"
		})).Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.False(t, result)
	})
}

func TestService_InitiatePayment_UpdateGatewayError(t *testing.T) {
	ctx := context.Background()
	orderID := uuid.New()
	params := payment.InitiatePaymentParams{
		OrderID:         orderID,
		Amount:          10000,
		Currency:        "USD",
		PaymentMethodID: "pm_test_123",
	}

	t.Run("UpdateGateway error is logged but does not fail", func(t *testing.T) {
		svc, repo, gw, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, core.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *payment.Payment) {
				p.ID = uuid.New()
			}).
			Return(nil)

		gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: "txn_gw_err",
				Status:        "success",
			}, nil)

		repo.EXPECT().UpdateGateway(mock.Anything, mock.AnythingOfType("uuid.UUID"), "txn_gw_err", mock.Anything).
			Return(errors.New("update gateway failed"))

		result, err := svc.InitiatePayment(ctx, params)

		require.NoError(t, err)
		assert.True(t, result.Charged)
	})

	t.Run("UpdatePaymentURL error is logged but does not fail", func(t *testing.T) {
		svc, repo, gw, _, _, _, _, _, _, _ := newTestService(t)

		repo.EXPECT().GetActiveByOrderID(mock.Anything, orderID).
			Return(nil, core.ErrNotFound)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*payment.Payment")).
			Run(func(_ context.Context, p *payment.Payment) {
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

		repo.EXPECT().UpdatePaymentURL(mock.Anything, mock.AnythingOfType("uuid.UUID"), "https://pay.example.com/url-err").
			Return(errors.New("url update failed"))

		result, err := svc.InitiatePayment(ctx, params)

		require.NoError(t, err)
		assert.False(t, result.Charged)
		assert.Equal(t, "https://pay.example.com/url-err", result.PaymentURL)
	})
}

func TestService_FinalizePaymentSuccess_MultipleItems(t *testing.T) {
	t.Run("sorts items by product ID before deducting", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, orders, orderGet, orderItems, inventory, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &payment.Payment{
			ID:       job.PaymentID,
			Amount:   20000,
			Currency: "USD",
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 20000,
				Currency:    "USD",
				Status:      "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusRequiresReview, payment.StatusCancelled}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment"}, "paid").
			Return(nil)

		productID1 := uuid.New()
		productID2 := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID2, Quantity: 1},
				{ProductID: productID1, Quantity: 2},
			}, nil)

		inventory.EXPECT().Deduct(mock.Anything, mock.Anything, mock.Anything).
			Return(nil).Times(2)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})
}

func TestService_RunCompensatingRefund_Error(t *testing.T) {
	t.Run("compensating refund CreateJob error is logged", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, orderGet, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionCharge,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:              job.PaymentID,
			OrderID:         job.OrderID,
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: "pm_123",
		}

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"awaiting_payment", "payment_processing"}, "payment_processing").
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

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{}, errors.New("db down"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRequiresReview,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusSuccess}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment", "cancelled", "expired", "paid"},
			"fulfillment_failed").
			Return(nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.Action == payment.ActionRefund && j.InventoryAction == "release"
		})).Return(errors.New("create job failed"))

		result := svc.ProcessJob(ctx, job)
		assert.True(t, result)
	})
}

func TestService_HandleWebhook(t *testing.T) {
	ctx := context.Background()

	t.Run("success event with already succeeded payment", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusSuccess,
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
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, paymentID, payment.StatusCancelled,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing}).
			Return(nil)

		repo.EXPECT().ClearPaymentURL(mock.Anything, paymentID).
			Return(nil)

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
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
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusProcessing,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, paymentID, payment.StatusCancelled,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing}).
			Return(nil)

		repo.EXPECT().ClearPaymentURL(mock.Anything, paymentID).
			Return(nil)

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
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
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		unknownID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, unknownID).
			Return(nil, core.ErrNotFound)

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
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()

		p := &payment.Payment{
			ID:     paymentID,
			Status: payment.StatusRefunded,
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
		svc, _, _, _, _, _, _, _, _, _ := newTestService(t)

		payload := map[string]any{
			"event": "success",
		}

		err := svc.HandleWebhook(ctx, payload)

		require.NoError(t, err)
	})

	t.Run("success event finalizes payment", func(t *testing.T) {
		svc, repo, _, orders, orderGet, orderItems, inv, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()
		productID := uuid.New()

		p := &payment.Payment{
			ID:       paymentID,
			OrderID:  orderID,
			Amount:   5000,
			Currency: "USD",
			Status:   payment.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil).Times(2)

		orderGet.EXPECT().GetByID(mock.Anything, orderID).Return(payment.OrderSnapshot{
			TotalAmount: 5000,
			Currency:    "USD",
			Status:      "awaiting_payment",
		}, nil)

		repo.EXPECT().MarkPaid(mock.Anything, paymentID,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusRequiresReview, payment.StatusCancelled}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, orderID,
			[]string{"payment_processing", "awaiting_payment"}, "paid").
			Return(nil)

		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, orderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 2},
			}, nil)

		inv.EXPECT().Deduct(mock.Anything, productID, 2).Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, mock.AnythingOfType("uuid.UUID")).Return(nil)
		repo.EXPECT().MarkJobCompletedByPaymentID(mock.Anything, paymentID, payment.ActionCharge).Return(nil)

		testCtx := database.WithTestTx(ctx, noopDBTX{})

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
	ctx := context.Background()

	t.Run("success", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		orderID := uuid.New()

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(nil)

		err := svc.CancelJobsByOrderID(ctx, orderID)

		require.NoError(t, err)
	})

	t.Run("error propagates", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		orderID := uuid.New()

		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).
			Return(errors.New("db error"))

		err := svc.CancelJobsByOrderID(ctx, orderID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "db error")
	})
}

func TestService_Refund(t *testing.T) {
	ctx := context.Background()

	t.Run("success with release inventory action", func(t *testing.T) {
		svc, repo, _, _, orderGet, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		orderGet.EXPECT().GetByID(mock.Anything, orderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
				Status:      "awaiting_payment",
			}, nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *payment.Job) bool {
			return job.PaymentID == paymentID &&
				job.OrderID == orderID &&
				job.Action == payment.ActionRefund &&
				job.Status == payment.JobStatusPending &&
				job.InventoryAction == "release"
		})).Return(nil)

		err := svc.Refund(ctx, paymentID)

		require.NoError(t, err)
	})

	t.Run("success with restock inventory action for paid order", func(t *testing.T) {
		svc, repo, _, _, orderGet, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		orderGet.EXPECT().GetByID(mock.Anything, orderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
				Status:      "paid",
			}, nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *payment.Job) bool {
			return job.InventoryAction == "restock"
		})).Return(nil)

		err := svc.Refund(ctx, paymentID)

		require.NoError(t, err)
	})

	t.Run("success with restock inventory action for delivered order", func(t *testing.T) {
		svc, repo, _, _, orderGet, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusRequiresReview,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		orderGet.EXPECT().GetByID(mock.Anything, orderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
				Status:      "delivered",
			}, nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *payment.Job) bool {
			return job.InventoryAction == "restock"
		})).Return(nil)

		err := svc.Refund(ctx, paymentID)

		require.NoError(t, err)
	})

	t.Run("payment not refundable - wrong status", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()

		p := &payment.Payment{
			ID:     paymentID,
			Status: payment.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("payment not refundable - cancelled status", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()

		p := &payment.Payment{
			ID:     paymentID,
			Status: payment.StatusCancelled,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("payment not found", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(nil, core.ErrNotFound)

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("order getter error", func(t *testing.T) {
		svc, repo, _, _, orderGet, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		orderGet.EXPECT().GetByID(mock.Anything, orderID).
			Return(payment.OrderSnapshot{}, errors.New("order service down"))

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "order service down")
	})

	t.Run("create job error", func(t *testing.T) {
		svc, repo, _, _, orderGet, _, _, _, _, _ := newTestService(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		orderGet.EXPECT().GetByID(mock.Anything, orderID).
			Return(payment.OrderSnapshot{
				Status: "awaiting_payment",
			}, nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.Anything).
			Return(errors.New("insert job failed"))

		err := svc.Refund(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "insert job failed")
	})
}

func TestService_FinalizePaymentSuccess(t *testing.T) {
	t.Run("success happy path", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, orders, orderGet, orderItems, inventory, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &payment.Payment{
			ID:       job.PaymentID,
			OrderID:  job.OrderID,
			Amount:   10000,
			Currency: "USD",
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
				Status:      "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusRequiresReview, payment.StatusCancelled}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment"}, "paid").
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 3},
			}, nil)

		inventory.EXPECT().Deduct(mock.Anything, productID, 3).
			Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("amount mismatch returns error", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, _, orderGet, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &payment.Payment{
			ID:       job.PaymentID,
			Amount:   5000,
			Currency: "USD",
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrAmountMismatch)
	})

	t.Run("currency mismatch returns error", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, _, orderGet, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &payment.Payment{
			ID:       job.PaymentID,
			Amount:   10000,
			Currency: "EUR",
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorIs(t, err, core.ErrAmountMismatch)
	})

	t.Run("already finalized by webhook", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, orders, orderGet, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &payment.Payment{
			ID:       job.PaymentID,
			Amount:   10000,
			Currency: "USD",
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusRequiresReview, payment.StatusCancelled}).
			Return(errors.New("already paid"))

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment"}, "paid").
			Return(errors.New("already paid"))

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.ErrorIs(t, err, core.ErrAlreadyFinalized)
	})

	t.Run("late payment enqueues refund job", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, orders, orderGet, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &payment.Payment{
			ID:       job.PaymentID,
			Amount:   10000,
			Currency: "USD",
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
				Status:      "cancelled",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusRequiresReview, payment.StatusCancelled}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment"}, "paid").
			Return(errors.New("order already cancelled"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRequiresReview,
			[]payment.Status{payment.StatusSuccess}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"cancelled", "expired", "paid"}, "fulfillment_failed").
			Return(nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.PaymentID == job.PaymentID &&
				j.OrderID == job.OrderID &&
				j.Action == payment.ActionRefund &&
				j.Status == payment.JobStatusPending &&
				j.InventoryAction == "release"
		})).Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("inventory deduction error propagates", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, orders, orderGet, orderItems, inventory, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &payment.Payment{
			ID:       job.PaymentID,
			Amount:   10000,
			Currency: "USD",
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
				Status:      "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusRequiresReview, payment.StatusCancelled}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment"}, "paid").
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 1},
			}, nil)

		inventory.EXPECT().Deduct(mock.Anything, productID, 1).
			Return(errors.New("out of stock"))

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "deducting inventory")
	})

	t.Run("order snapshot error propagates", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, _, _, _, orderGet, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{}, errors.New("db down"))

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "getting order for verification")
	})

	t.Run("payment get error propagates", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, _, orderGet, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("payment not found"))

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "getting payment for verification")
	})

	t.Run("late payment with paid order uses restock inventory action", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, orders, orderGet, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &payment.Payment{
			ID:       job.PaymentID,
			Amount:   10000,
			Currency: "USD",
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
				Status:      "paid",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusRequiresReview, payment.StatusCancelled}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment"}, "paid").
			Return(errors.New("already paid"))

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRequiresReview,
			[]payment.Status{payment.StatusSuccess}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"cancelled", "expired", "paid"}, "fulfillment_failed").
			Return(nil)

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.InventoryAction == "restock"
		})).Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.NoError(t, err)
	})

	t.Run("listing order items error propagates", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, _, orders, orderGet, orderItems, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
		}

		p := &payment.Payment{
			ID:       job.PaymentID,
			Amount:   10000,
			Currency: "USD",
		}

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 10000,
				Currency:    "USD",
				Status:      "awaiting_payment",
			}, nil)

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().MarkPaid(mock.Anything, job.PaymentID,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusRequiresReview, payment.StatusCancelled}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"payment_processing", "awaiting_payment"}, "paid").
			Return(nil)

		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return(nil, errors.New("items db error"))

		err := svc.FinalizePaymentSuccess(ctx, job)
		require.Error(t, err)
		assert.ErrorContains(t, err, "listing order items")
	})
}

func TestService_ProcessRefundJob(t *testing.T) {
	t.Run("success with release inventory", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRel, _, couponRel := newTestService(t)

		job := payment.Job{
			ID:              uuid.New(),
			PaymentID:       uuid.New(),
			OrderID:         uuid.New(),
			Action:          payment.ActionRefund,
			Attempts:        0,
			MaxAttempts:     3,
			InventoryAction: "release",
		}

		p := &payment.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       5000,
			GatewayTxnID: "txn_123",
			Status:       payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, gateway.RefundRequest{
			TransactionID: "txn_123",
			Amount:        5000,
			Reason:        "auto-refund",
		}).Return(gateway.RefundResponse{RefundID: "ref_001"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRefunded,
			[]payment.Status{payment.StatusSuccess, payment.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"fulfillment_failed", "paid", "delivered"}, "refunded").
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 2},
			}, nil)

		inventoryRel.EXPECT().Release(mock.Anything, productID, 2).
			Return(nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{CouponCode: "SAVE10"}, nil)

		couponRel.EXPECT().Release(mock.Anything, job.OrderID).
			Return(nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.True(t, result)
	})

	t.Run("success with restock inventory", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, orderGet, orderItems, _, _, inventoryRestock, _ := newTestService(t)

		job := payment.Job{
			ID:              uuid.New(),
			PaymentID:       uuid.New(),
			OrderID:         uuid.New(),
			Action:          payment.ActionRefund,
			Attempts:        0,
			MaxAttempts:     3,
			InventoryAction: "restock",
		}

		p := &payment.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       8000,
			GatewayTxnID: "txn_456",
			Status:       payment.StatusRequiresReview,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, gateway.RefundRequest{
			TransactionID: "txn_456",
			Amount:        8000,
			Reason:        "auto-refund",
		}).Return(gateway.RefundResponse{RefundID: "ref_002"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRefunded,
			[]payment.Status{payment.StatusSuccess, payment.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"fulfillment_failed", "paid", "delivered"}, "refunded").
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 5},
			}, nil)

		inventoryRestock.EXPECT().Restock(mock.Anything, productID, 5).
			Return(nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{}, nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.True(t, result)
	})

	t.Run("payment not refundable", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    payment.ActionRefund,
		}

		p := &payment.Payment{
			ID:     job.PaymentID,
			Status: payment.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.ID == job.ID && j.Status == payment.JobStatusCancelled
		})).Return(nil)

		result := svc.ProcessJob(context.Background(), job)
		assert.False(t, result)
	})

	t.Run("payment not found", func(t *testing.T) {
		svc, repo, _, _, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    payment.ActionRefund,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("not found"))

		result := svc.ProcessJob(context.Background(), job)
		assert.False(t, result)
	})

	t.Run("gateway refund error with retries remaining", func(t *testing.T) {
		svc, repo, gw, _, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:           job.PaymentID,
			Amount:       5000,
			GatewayTxnID: "txn_789",
			Status:       payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{}, errors.New("gateway timeout"))

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.ID == job.ID &&
				j.Status == payment.JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway timeout"
		})).Return(nil)

		result := svc.ProcessJob(context.Background(), job)
		assert.False(t, result)
	})

	t.Run("gateway refund error max attempts", func(t *testing.T) {
		svc, repo, gw, _, _, _, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      payment.ActionRefund,
			Attempts:    2,
			MaxAttempts: 3,
		}

		p := &payment.Payment{
			ID:           job.PaymentID,
			Amount:       5000,
			GatewayTxnID: "txn_999",
			Status:       payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{}, errors.New("gateway error"))

		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *payment.Job) bool {
			return j.ID == job.ID &&
				j.Status == payment.JobStatusFailed &&
				j.Attempts == 3
		})).Return(nil)

		result := svc.ProcessJob(context.Background(), job)
		assert.False(t, result)
	})

	t.Run("refund with list items error returns false", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, _, orderItems, _, _, _, _ := newTestService(t)

		job := payment.Job{
			ID:              uuid.New(),
			PaymentID:       uuid.New(),
			OrderID:         uuid.New(),
			Action:          payment.ActionRefund,
			Attempts:        0,
			MaxAttempts:     3,
			InventoryAction: "release",
		}

		p := &payment.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       5000,
			GatewayTxnID: "txn_items_err",
			Status:       payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_items_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRefunded,
			[]payment.Status{payment.StatusSuccess, payment.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"fulfillment_failed", "paid", "delivered"}, "refunded").
			Return(nil)

		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return(nil, errors.New("db error"))

		result := svc.ProcessJob(ctx, job)
		assert.False(t, result)
	})

	t.Run("refund with multiple items sorts by product ID", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRel, _, _ := newTestService(t)

		job := payment.Job{
			ID:              uuid.New(),
			PaymentID:       uuid.New(),
			OrderID:         uuid.New(),
			Action:          payment.ActionRefund,
			Attempts:        0,
			MaxAttempts:     3,
			InventoryAction: "release",
		}

		p := &payment.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       5000,
			GatewayTxnID: "txn_multi",
			Status:       payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_multi"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRefunded,
			[]payment.Status{payment.StatusSuccess, payment.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"fulfillment_failed", "paid", "delivered"}, "refunded").
			Return(nil)

		productID1 := uuid.New()
		productID2 := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID2, Quantity: 1},
				{ProductID: productID1, Quantity: 2},
			}, nil)

		inventoryRel.EXPECT().Release(mock.Anything, mock.Anything, mock.Anything).
			Return(nil).Times(2)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{}, nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.True(t, result)
	})

	t.Run("refund with release inventory error logs but continues", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRel, _, _ := newTestService(t)

		job := payment.Job{
			ID:              uuid.New(),
			PaymentID:       uuid.New(),
			OrderID:         uuid.New(),
			Action:          payment.ActionRefund,
			Attempts:        0,
			MaxAttempts:     3,
			InventoryAction: "release",
		}

		p := &payment.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       5000,
			GatewayTxnID: "txn_rel_err",
			Status:       payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_rel_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRefunded,
			[]payment.Status{payment.StatusSuccess, payment.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"fulfillment_failed", "paid", "delivered"}, "refunded").
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 1},
			}, nil)

		inventoryRel.EXPECT().Release(mock.Anything, productID, 1).
			Return(errors.New("release failed"))

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{}, nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.True(t, result)
	})

	t.Run("refund with restock inventory error logs but continues", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, orderGet, orderItems, _, _, inventoryRestock, _ := newTestService(t)

		job := payment.Job{
			ID:              uuid.New(),
			PaymentID:       uuid.New(),
			OrderID:         uuid.New(),
			Action:          payment.ActionRefund,
			Attempts:        0,
			MaxAttempts:     3,
			InventoryAction: "restock",
		}

		p := &payment.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       5000,
			GatewayTxnID: "txn_restock_err",
			Status:       payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_restock_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRefunded,
			[]payment.Status{payment.StatusSuccess, payment.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"fulfillment_failed", "paid", "delivered"}, "refunded").
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 1},
			}, nil)

		inventoryRestock.EXPECT().Restock(mock.Anything, productID, 1).
			Return(errors.New("restock failed"))

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{}, nil)

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.True(t, result)
	})

	t.Run("refund with coupon release error logs but continues", func(t *testing.T) {
		ctx := database.WithTestTx(context.Background(), noopDBTX{})
		svc, repo, gw, orders, orderGet, orderItems, _, inventoryRel, _, couponRel := newTestService(t)

		job := payment.Job{
			ID:              uuid.New(),
			PaymentID:       uuid.New(),
			OrderID:         uuid.New(),
			Action:          payment.ActionRefund,
			Attempts:        0,
			MaxAttempts:     3,
			InventoryAction: "release",
		}

		p := &payment.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       5000,
			GatewayTxnID: "txn_coupon_err",
			Status:       payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_coupon_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, payment.StatusRefunded,
			[]payment.Status{payment.StatusSuccess, payment.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().UpdateStatus(mock.Anything, job.OrderID,
			[]string{"fulfillment_failed", "paid", "delivered"}, "refunded").
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemsByOrderID(mock.Anything, job.OrderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 1},
			}, nil)

		inventoryRel.EXPECT().Release(mock.Anything, productID, 1).
			Return(nil)

		orderGet.EXPECT().GetByID(mock.Anything, job.OrderID).
			Return(payment.OrderSnapshot{CouponCode: "SAVE10"}, nil)

		couponRel.EXPECT().Release(mock.Anything, job.OrderID).
			Return(errors.New("coupon release failed"))

		repo.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		result := svc.ProcessJob(ctx, job)
		assert.True(t, result)
	})
}
