package e2e_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	mockgatewayserver "github.com/residwi/go-api-project-template/cmd/mockgateway/mockserver"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// Order total and payment amount must be the same number or
// FinalizeSuccess rejects the charge as a mismatch. 1000 is also not
// 99 mod 100, which the mock gateway declines.
const (
	chargeAmount = 1000

	// The split a placed-but-not-charged order leaves: its unit is held, not sold.
	chargeAvailableStock = 99
	chargeReservedStock  = 1
)

func TestE2EChargeJob(t *testing.T) {
	setup(t)
	ctx := context.Background()

	t.Run("processes a pending charge job to completion", func(t *testing.T) {
		mockMux := http.NewServeMux()
		mockgatewayserver.RegisterRoutes(mockMux, testhelper.DiscardLogger())
		mockServer := httptest.NewServer(mockMux)
		defer mockServer.Close()

		f := seedChargeJob(t, 3)

		require.NoError(t, newPaymentService(t, mockServer.URL+"/mock/payment").JobProcessor.Process(ctx, f.job))

		assert.Equal(t, string(domain.JobStatusCompleted), jobStatusOf(t, f.job.ID))
		assert.Equal(t, string(domain.StatusSuccess), paymentStatusOf(t, f.paymentID))
		assert.Equal(t, "paid", orderStatusOf(t, f.job.OrderID))

		// A Deduct on the wrong column would still affect one row and return nil,
		// so only this available/reserved split says which column moved.
		available, reserved := inventoryLevelOf(t, f.productID)
		assert.Equal(t, chargeAvailableStock, available)
		assert.Equal(t, 0, reserved)
	})

	t.Run("marks the job failed after the final gateway failure", func(t *testing.T) {
		// 500s everything under the prefix mock.Gateway posts to, because the real mock
		// gateway cannot be told to fail a charge outright and handleChargeFailure keys
		// off the resulting non-200.
		failingMux := http.NewServeMux()
		failingMux.HandleFunc("/mock/payment/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		failingServer := httptest.NewServer(failingMux)
		defer failingServer.Close()

		// max_attempts=1, so this first failure is also the final one.
		f := seedChargeJob(t, 1)

		err := newPaymentService(t, failingServer.URL+"/mock/payment").JobProcessor.Process(ctx, f.job)
		require.Error(t, err)

		assert.Equal(t, string(domain.JobStatusFailed), jobStatusOf(t, f.job.ID))

		// handleChargeFailure logs MarkAwaitingPayment's error and moves on, so a broken
		// CAS would strand the order in payment_processing with every other assertion
		// here still green.
		assert.Equal(t, "awaiting_payment", orderStatusOf(t, f.job.OrderID))
	})
}

type chargeJobFixture struct {
	paymentID uuid.UUID
	productID uuid.UUID
	job       domain.Job
}

// SQL rather than checkout, because checkout cannot leave an order where
// processChargeJob wants it: a payment_method_id charges synchronously, so the
// order comes back 'paid' and Process would cancel the job instead.
//
// What the fixture has to satisfy: orders.status 'awaiting_payment' for the
// claiming CAS, a non-empty payment_method_id or the gateway answers "pending",
// payments.amount equal to orders.total_amount or FinalizeSuccess bails
// with ErrAmountMismatch, payments.status 'pending', and an order_items row over
// reserved stock for Deduct to consume.
func seedChargeJob(t *testing.T, maxAttempts int) chargeJobFixture {
	t.Helper()
	ctx := context.Background()

	userID := testhelper.SeedUser(t, testPool)

	prodID := uuid.New()
	_, err := testPool.Exec(ctx,
		`INSERT INTO products (id, name, slug, description, price, currency, status)
		 VALUES ($1, 'Charge Product', $2, 'desc', $3, 'USD', 'published')`,
		prodID, "charge-prod-"+prodID.String()[:8], chargeAmount)
	require.NoError(t, err)
	seedInventoryLevel(t, prodID, chargeAvailableStock, chargeReservedStock)

	orderID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO orders (id, user_id, idempotency_key, request_hash, status,
		                     subtotal_amount, discount_amount, total_amount, currency, notes)
		 VALUES ($1, $2, $3, '', 'awaiting_payment', $4, 0, $4, 'USD', '')`,
		orderID, userID, orderID.String(), chargeAmount)
	require.NoError(t, err)

	_, err = testPool.Exec(ctx,
		`INSERT INTO order_items (order_id, product_id, product_name, price, quantity, subtotal)
		 VALUES ($1, $2, 'Charge Product', $3, $4, $3)`,
		orderID, prodID, chargeAmount, chargeReservedStock)
	require.NoError(t, err)

	paymentID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO payments (id, order_id, amount, currency, status, method, payment_method_id)
		 VALUES ($1, $2, $3, 'USD', 'pending', 'card', 'pm_test_123')`,
		paymentID, orderID, chargeAmount)
	require.NoError(t, err)

	jobID := uuid.New()
	_, err = testPool.Exec(ctx,
		`INSERT INTO payment_jobs (id, payment_id, order_id, action, status, max_attempts, next_retry_at)
		 VALUES ($1, $2, $3, 'charge', 'pending', $4, NOW())`,
		jobID, paymentID, orderID, maxAttempts)
	require.NoError(t, err)

	// Read back rather than built in Go, so Process runs over the row the database
	// holds, defaults included.
	var job domain.Job
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT id, payment_id, order_id, action, status, attempts, max_attempts,
		        COALESCE(last_error, ''), locked_until, next_retry_at,
		        created_at, updated_at
		 FROM payment_jobs WHERE id = $1`, jobID).Scan(
		&job.ID, &job.PaymentID, &job.OrderID, &job.Action, &job.Status,
		&job.Attempts, &job.MaxAttempts, &job.LastError, &job.LockedUntil,
		&job.NextRetryAt, &job.CreatedAt, &job.UpdatedAt,
	))

	return chargeJobFixture{paymentID: paymentID, productID: prodID, job: job}
}

func jobStatusOf(t *testing.T, jobID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT status FROM payment_jobs WHERE id = $1`, jobID).Scan(&status))
	return status
}

func paymentStatusOf(t *testing.T, paymentID uuid.UUID) string {
	t.Helper()
	var status string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT status FROM payments WHERE id = $1`, paymentID).Scan(&status))
	return status
}
