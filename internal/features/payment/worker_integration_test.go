package payment_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/payment"
	gateway "github.com/residwi/go-api-project-template/internal/platform/payment"
	mocks "github.com/residwi/go-api-project-template/mocks/payment"
)

func seedPaymentJob(t *testing.T, paymentID, orderID uuid.UUID, action payment.JobAction) *payment.Job {
	t.Helper()
	repo := payment.NewPostgresRepository(testPool)
	job := &payment.Job{
		PaymentID:   paymentID,
		OrderID:     orderID,
		Action:      action,
		Status:      payment.JobStatusPending,
		MaxAttempts: 3,
		NextRetryAt: time.Now(),
	}
	err := repo.CreateJob(context.Background(), job)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })
	return job
}

func workerConfig() payment.WorkerConfig {
	return payment.WorkerConfig{
		Interval:      20 * time.Millisecond,
		BatchSize:     10,
		LeaseDuration: 30 * time.Second,
		Concurrency:   2,
	}
}

func TestWorker_SweepExpiredOrders(t *testing.T) {
	t.Run("expires orders older than 30 minutes", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)

		_, err := testPool.Exec(context.Background(),
			`UPDATE orders SET created_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, orderID)
		require.NoError(t, err)

		orderUpdater := mocks.NewMockOrderUpdater(t)
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, orderID, []string{"awaiting_payment"}, "expired").
			Return(nil)
		// other expired orders from parallel tests
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Maybe().
			Return(nil)

		repo := payment.NewPostgresRepository(testPool)
		orderItems := mocks.NewMockOrderItemsGetter(t)
		orderGet := mocks.NewMockOrderGetter(t)
		inventoryRel := mocks.NewMockInventoryReleaser(t)
		couponRel := mocks.NewMockCouponReleaser(t)
		gw := mocks.NewMockGateway(t)
		inventoryDeduct := mocks.NewMockInventoryDeductor(t)
		inventoryRestock := mocks.NewMockInventoryRestocker(t)

		svc := payment.NewService(repo, testPool, gw, orderUpdater, orderGet, orderItems,
			inventoryDeduct, inventoryRel, inventoryRestock, couponRel)

		w := payment.NewWorker(repo, testPool, svc, orderUpdater, orderItems, orderGet,
			inventoryRel, couponRel, workerConfig())

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		w.Start(ctx)

		orderUpdater.AssertExpectations(t)
	})

	t.Run("does not expire recent orders", func(t *testing.T) {
		userID := seedUser(t)
		seedOrder(t, userID)

		orderUpdater := mocks.NewMockOrderUpdater(t)

		repo := payment.NewPostgresRepository(testPool)
		orderItems := mocks.NewMockOrderItemsGetter(t)
		orderGet := mocks.NewMockOrderGetter(t)
		inventoryRel := mocks.NewMockInventoryReleaser(t)
		couponRel := mocks.NewMockCouponReleaser(t)
		gw := mocks.NewMockGateway(t)
		inventoryDeduct := mocks.NewMockInventoryDeductor(t)
		inventoryRestock := mocks.NewMockInventoryRestocker(t)

		svc := payment.NewService(repo, testPool, gw, orderUpdater, orderGet, orderItems,
			inventoryDeduct, inventoryRel, inventoryRestock, couponRel)

		w := payment.NewWorker(repo, testPool, svc, orderUpdater, orderItems, orderGet,
			inventoryRel, couponRel, workerConfig())

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		w.Start(ctx)

		orderUpdater.AssertNotCalled(t, "UpdateStatus")
	})
}

func TestWorker_SweepExpiredOrders_UpdateStatusError(t *testing.T) {
	t.Run("logs warning when UpdateStatus fails for expired order", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)

		_, err := testPool.Exec(context.Background(),
			`UPDATE orders SET created_at = NOW() - INTERVAL '1 hour' WHERE id = $1`, orderID)
		require.NoError(t, err)

		orderUpdater := mocks.NewMockOrderUpdater(t)
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, orderID, []string{"awaiting_payment"}, "expired").
			Return(assert.AnError)
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Maybe().
			Return(nil)

		repo := payment.NewPostgresRepository(testPool)
		orderItems := mocks.NewMockOrderItemsGetter(t)
		orderGet := mocks.NewMockOrderGetter(t)
		inventoryRel := mocks.NewMockInventoryReleaser(t)
		couponRel := mocks.NewMockCouponReleaser(t)
		gw := mocks.NewMockGateway(t)
		inventoryDeduct := mocks.NewMockInventoryDeductor(t)
		inventoryRestock := mocks.NewMockInventoryRestocker(t)

		svc := payment.NewService(repo, testPool, gw, orderUpdater, orderGet, orderItems,
			inventoryDeduct, inventoryRel, inventoryRestock, couponRel)

		w := payment.NewWorker(repo, testPool, svc, orderUpdater, orderItems, orderGet,
			inventoryRel, couponRel, workerConfig())

		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		w.Start(ctx)

		orderUpdater.AssertExpectations(t)
	})
}

func TestWorker_CleanupOldJobs(t *testing.T) {
	t.Run("deletes completed jobs older than 7 days", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)

		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		job := &payment.Job{
			PaymentID:   p.ID,
			OrderID:     orderID,
			Action:      payment.ActionCharge,
			Status:      payment.JobStatusPending,
			MaxAttempts: 3,
			NextRetryAt: time.Now(),
		}
		err := repo.CreateJob(ctx, job)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		err = repo.MarkJobCompleted(ctx, job.ID)
		require.NoError(t, err)

		// Disable trigger so we can backdate updated_at (trigger auto-sets NOW())
		_, err = testPool.Exec(ctx, `ALTER TABLE payment_jobs DISABLE TRIGGER update_payment_jobs_updated_at`)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx,
			`UPDATE payment_jobs SET updated_at = NOW() - INTERVAL '8 days' WHERE id = $1`, job.ID)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `ALTER TABLE payment_jobs ENABLE TRIGGER update_payment_jobs_updated_at`)
		require.NoError(t, err)

		orderUpdater := mocks.NewMockOrderUpdater(t)
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Maybe().
			Return(nil)
		orderItems := mocks.NewMockOrderItemsGetter(t)
		orderGet := mocks.NewMockOrderGetter(t)
		inventoryRel := mocks.NewMockInventoryReleaser(t)
		couponRel := mocks.NewMockCouponReleaser(t)
		gw := mocks.NewMockGateway(t)
		inventoryDeduct := mocks.NewMockInventoryDeductor(t)
		inventoryRestock := mocks.NewMockInventoryRestocker(t)

		svc := payment.NewService(repo, testPool, gw, orderUpdater, orderGet, orderItems,
			inventoryDeduct, inventoryRel, inventoryRestock, couponRel)

		w := payment.NewWorker(repo, testPool, svc, orderUpdater, orderItems, orderGet,
			inventoryRel, couponRel, workerConfig())

		wCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		w.Start(wCtx)

		var count int
		err = testPool.QueryRow(ctx, `SELECT COUNT(*) FROM payment_jobs WHERE id = $1`, job.ID).Scan(&count)
		require.NoError(t, err)
		assert.Equal(t, 0, count)
	})
}

func TestWorker_ProcessPaymentJobs(t *testing.T) {
	t.Run("processes pending charge job to completion", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		job := seedPaymentJob(t, p.ID, orderID, payment.ActionCharge)

		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		orderUpdater := mocks.NewMockOrderUpdater(t)
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, orderID, []string{"awaiting_payment", "payment_processing"}, "payment_processing").
			Return(nil)
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, orderID, []string{"payment_processing", "awaiting_payment"}, "paid").
			Return(nil)

		orderGet := mocks.NewMockOrderGetter(t)
		orderGet.EXPECT().
			GetByID(mock.Anything, orderID).
			Return(payment.OrderSnapshot{
				TotalAmount: 1000,
				Currency:    "USD",
				Status:      "awaiting_payment",
			}, nil)

		orderItems := mocks.NewMockOrderItemsGetter(t)
		productID := uuid.New()
		orderItems.EXPECT().
			ListItemsByOrderID(mock.Anything, orderID).
			Return([]payment.OrderItemDTO{
				{ProductID: productID, Quantity: 1},
			}, nil)

		inventoryDeduct := mocks.NewMockInventoryDeductor(t)
		inventoryDeduct.EXPECT().
			Deduct(mock.Anything, productID, 1).
			Return(nil)

		gw := mocks.NewMockGateway(t)
		txnID := "txn-" + uuid.New().String()
		gw.EXPECT().
			Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{
				TransactionID: txnID,
				Status:        "success",
			}, nil)

		inventoryRel := mocks.NewMockInventoryReleaser(t)
		inventoryRestock := mocks.NewMockInventoryRestocker(t)
		couponRel := mocks.NewMockCouponReleaser(t)

		svc := payment.NewService(repo, testPool, gw, orderUpdater, orderGet, orderItems,
			inventoryDeduct, inventoryRel, inventoryRestock, couponRel)

		w := payment.NewWorker(repo, testPool, svc, orderUpdater, orderItems, orderGet,
			inventoryRel, couponRel, workerConfig())

		wCtx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		defer cancel()

		w.Start(wCtx)

		var jobStatus string
		err := testPool.QueryRow(ctx,
			`SELECT status FROM payment_jobs WHERE id = $1`, job.ID).Scan(&jobStatus)
		require.NoError(t, err)
		assert.Equal(t, string(payment.JobStatusCompleted), jobStatus)

		var paymentStatus string
		err = testPool.QueryRow(ctx,
			`SELECT status FROM payments WHERE id = $1`, p.ID).Scan(&paymentStatus)
		require.NoError(t, err)
		assert.Equal(t, string(payment.StatusSuccess), paymentStatus)
	})

	t.Run("retries on gateway failure and marks failed after max attempts", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)

		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		job := &payment.Job{
			PaymentID:   p.ID,
			OrderID:     orderID,
			Action:      payment.ActionCharge,
			Status:      payment.JobStatusPending,
			MaxAttempts: 1,
			NextRetryAt: time.Now(),
		}
		err := repo.CreateJob(ctx, job)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		// CreateJob doesn't insert max_attempts; set it explicitly so one failure hits the limit
		_, err = testPool.Exec(ctx, `UPDATE payment_jobs SET max_attempts = 1 WHERE id = $1`, job.ID)
		require.NoError(t, err)

		orderUpdater := mocks.NewMockOrderUpdater(t)
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, orderID, []string{"awaiting_payment", "payment_processing"}, "payment_processing").
			Return(nil)
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, orderID, []string{"payment_processing"}, "awaiting_payment").
			Return(nil)
		// sweepExpiredOrders may find other orders
		orderUpdater.EXPECT().
			UpdateStatus(mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Maybe().
			Return(nil)

		gw := mocks.NewMockGateway(t)
		gw.EXPECT().
			Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, assert.AnError)

		orderGet := mocks.NewMockOrderGetter(t)
		orderItems := mocks.NewMockOrderItemsGetter(t)
		inventoryDeduct := mocks.NewMockInventoryDeductor(t)
		inventoryRel := mocks.NewMockInventoryReleaser(t)
		inventoryRestock := mocks.NewMockInventoryRestocker(t)
		couponRel := mocks.NewMockCouponReleaser(t)

		svc := payment.NewService(repo, testPool, gw, orderUpdater, orderGet, orderItems,
			inventoryDeduct, inventoryRel, inventoryRestock, couponRel)

		w := payment.NewWorker(repo, testPool, svc, orderUpdater, orderItems, orderGet,
			inventoryRel, couponRel, workerConfig())

		wCtx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		w.Start(wCtx)

		var jobStatus string
		err = testPool.QueryRow(ctx,
			`SELECT status FROM payment_jobs WHERE id = $1`, job.ID).Scan(&jobStatus)
		require.NoError(t, err)
		assert.Equal(t, string(payment.JobStatusFailed), jobStatus)
	})
}
