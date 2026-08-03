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
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

// Amounts and stock the charge fixture is built from. The order total and the
// payment amount must be the same number or FinalizePaymentSuccess rejects the
// charge as a mismatch, so both read it from here. 1000 is also deliberately
// not congruent to 99 mod 100: the mock gateway declines those (see
// cmd/mockgateway/mockserver/gateway.go).
const (
	chargeAmount = 1000

	// chargeAvailableStock / chargeReservedStock is the split an order that has
	// been placed but not yet charged leaves behind: its unit is held, not sold.
	chargeAvailableStock = 99
	chargeReservedStock  = 1
)

// TestE2EChargeJob drives payment.Service.Process over an action='charge' job
// with every collaborator real: the order service applies the status
// transitions, the inventory service consumes the reservation inside the
// finalizing transaction, and the charge leaves the process over HTTP.
//
// It used to live in internal/modules/payment as a service-level integration
// test — a real Postgres repository wired to six mocked collaborators — which
// forced that package to import its own database adapter. Whether the service
// calls its ports in the right order is already service_test.go's subject;
// what is left here is whether a charge job actually settles.
//
// The job row is hand-inserted because nothing in production enqueues one:
// every CreateJob call site in the payment package enqueues ActionRefund, and
// a card charge finalizes synchronously at placement instead. See
// ARCHITECTURE-LIMITATIONS.md and the note in fulfillment_failed_test.go.
func TestE2EChargeJob(t *testing.T) {
	setup(t)
	ctx := context.Background()

	t.Run("processes a pending charge job to completion", func(t *testing.T) {
		mockMux := http.NewServeMux()
		mockgatewayserver.RegisterRoutes(mockMux)
		mockServer := httptest.NewServer(mockMux)
		defer mockServer.Close()

		f := seedChargeJob(t, 3)

		require.NoError(t, newPaymentService(t, mockServer.URL+"/mock/payment").Process(ctx, f.job))

		assert.Equal(t, string(payment.JobStatusCompleted), jobStatusOf(t, f.job.ID))
		assert.Equal(t, string(payment.StatusSuccess), paymentStatusOf(t, f.paymentID))
		assert.Equal(t, order.StatusPaid, orderStatusOf(t, f.job.OrderID))

		// The unit this order was holding is now sold rather than reserved, and
		// nothing returns to the shelf, so available_stock is untouched. A
		// DeductBatch that touched the wrong column would still affect one row and
		// return nil, so no assertion above would see it -- this split is what
		// pins down which column moved.
		available, reserved := inventoryLevelOf(t, f.productID)
		assert.Equal(t, chargeAvailableStock, available)
		assert.Equal(t, 0, reserved)
	})

	t.Run("marks the job failed after the final gateway failure", func(t *testing.T) {
		// A gateway that 500s everything under /mock/payment/ — the prefix the
		// real mock gateway registers its POST routes on, so mock.Gateway's
		// baseURL+"/charge" lands here. mock.Gateway turns any non-200 into an
		// error, which is the branch handleChargeFailure keys off; the real mock
		// gateway has no way to be told to fail a charge outright.
		failingMux := http.NewServeMux()
		failingMux.HandleFunc("/mock/payment/", func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		})
		failingServer := httptest.NewServer(failingMux)
		defer failingServer.Close()

		// max_attempts=1, so this first failure is also the final one.
		f := seedChargeJob(t, 1)

		err := newPaymentService(t, failingServer.URL+"/mock/payment").Process(ctx, f.job)
		require.Error(t, err)

		assert.Equal(t, string(payment.JobStatusFailed), jobStatusOf(t, f.job.ID))

		// handleChargeFailure swallows MarkAwaitingPayment's error -- it logs and
		// moves on (service.go:251-254) -- so a broken CAS would strand the order
		// in payment_processing with every other assertion here still green. This
		// is the only thing that would notice.
		assert.Equal(t, order.StatusAwaitingPayment, orderStatusOf(t, f.job.OrderID))
	})
}

// chargeJobFixture is an order sitting where a charge worker would find one,
// plus the pending job pointed at it.
type chargeJobFixture struct {
	paymentID uuid.UUID
	productID uuid.UUID
	job       payment.Job
}

// seedChargeJob writes the order/payment/job trio with SQL rather than driving
// checkout, because checkout cannot leave an order where processChargeJob wants
// it: placing an order with a payment_method_id charges synchronously and
// finalizes on the spot, so the order comes back 'paid' and the payment
// 'success'. MarkPaymentProcessing would then find nothing to claim and Process
// would cancel the job instead of completing it.
//
// What processChargeJob needs of this fixture, in the order it asks for it:
//
//   - orders.status = 'awaiting_payment', so PaymentProcessingTransition's CAS
//     matches and the order can be claimed for charging;
//   - payments.payment_method_id non-empty, or the mock gateway answers
//     "pending" instead of "success" and the charge counts as a failure;
//   - payments.amount = orders.total_amount, or FinalizePaymentSuccess bails
//     out with ErrAmountMismatch;
//   - payments.status = 'pending', one of the four states MarkPaid will CAS
//     from;
//   - an order_items row over a product with reserved stock, so the DeductBatch
//     inside the finalizing transaction has a reservation to consume.
//
// The nullable text columns are written as empty strings rather than left NULL
// to match what the order repository's own INSERT stores; its GetByID scans
// them into plain Go strings, which a NULL cannot fill.
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

	// Read the job back rather than building one in Go, so Process runs over the
	// row the database actually holds — including the defaults it filled in.
	var job payment.Job
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
