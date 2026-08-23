package postgres

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/money"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_payment")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

// Claim requires attempts < max_attempts, so a stored zero would strand the job
// forever.
func TestPostgresRepository_CreateJob_MaxAttempts(t *testing.T) {
	maxAttemptsOf := func(t *testing.T, jobID uuid.UUID) int {
		t.Helper()
		var got int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT max_attempts FROM payment_jobs WHERE id = $1`, jobID).Scan(&got))
		return got
	}

	t.Run("persists the caller's max_attempts", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		paymentID := seedPayment(t, orderID)
		repo := New(testPool)

		job := &domain.Job{
			PaymentID:   paymentID,
			OrderID:     orderID,
			Action:      domain.ActionCharge,
			Status:      domain.JobStatusPending,
			MaxAttempts: 7,
			NextRetryAt: time.Now(),
		}
		require.NoError(t, repo.CreateJob(context.Background(), job))
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		assert.Equal(t, 7, maxAttemptsOf(t, job.ID))
	})

	t.Run("falls back to the default when the caller leaves it unset", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		paymentID := seedPayment(t, orderID)
		repo := New(testPool)

		job := &domain.Job{
			PaymentID:   paymentID,
			OrderID:     orderID,
			Action:      domain.ActionRefund,
			Status:      domain.JobStatusPending,
			NextRetryAt: time.Now(),
		}
		require.NoError(t, repo.CreateJob(context.Background(), job))
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		assert.Positive(t, maxAttemptsOf(t, job.ID), "an unset max_attempts must not strand the job")
		assert.Equal(t, maxAttemptsOf(t, job.ID), job.MaxAttempts,
			"the caller should see the value that was actually stored")
	})
}

func TestPostgresRepository_JobLifecycle(t *testing.T) {
	t.Run("create, claim, update, cancel, complete, and delete job", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		paymentID := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		job := &domain.Job{
			PaymentID:   paymentID,
			OrderID:     orderID,
			Action:      domain.ActionCharge,
			Status:      domain.JobStatusPending,
			MaxAttempts: 3,
			// Claim sorts due jobs oldest-next_retry_at-first with a LIMIT, over
			// test_payment, a shared database that is never truncated (see the
			// registry comment in internal/testhelper/testhelper.go). 100 years
			// is arbitrary -- it only needs to predate every job that has ever
			// accumulated so this one is always claimed first and never crowded
			// out of the LIMIT, while still landing on the next_retry_at <= NOW()
			// boundary the claim query tests.
			NextRetryAt: time.Now().AddDate(-100, 0, 0),
		}
		err := repo.CreateJob(ctx, job)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, job.ID)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		claimed, err := repo.Claim(ctx, 10, 30*time.Second)
		require.NoError(t, err)
		require.NotEmpty(t, claimed, "expected to claim the seeded job")

		var claimedJob *domain.Job
		for i := range claimed {
			if claimed[i].ID == job.ID {
				claimedJob = &claimed[i]
			}
		}
		require.NotNil(t, claimedJob, "seeded job must be among the claimed batch")
		assert.Equal(t, domain.JobStatusProcessing, claimedJob.Status)
		assert.NotNil(t, claimedJob.LockedUntil)

		claimedJob.Attempts = 1
		claimedJob.LastError = "transient error"
		claimedJob.Status = domain.JobStatusProcessing
		claimedJob.NextRetryAt = time.Now().Add(5 * time.Second)
		err = repo.UpdateJob(ctx, claimedJob)
		require.NoError(t, err)

		err = repo.CancelJobsByOrderID(ctx, orderID)
		require.NoError(t, err)

		// MarkJobCompleted -- idempotent even after cancel
		err = repo.MarkJobCompleted(ctx, job.ID)
		require.NoError(t, err)

		// Prune with olderThan=0 so all completed/failed/cancelled qualify
		deleted, err := repo.Prune(ctx, 0, 100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, deleted, 1)
	})
}

func TestPostgresRepository_Claim_WithOptionalFields(t *testing.T) {
	t.Run("claimed job round-trips last_error", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		paymentID := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		job := &domain.Job{
			PaymentID:   paymentID,
			OrderID:     orderID,
			Action:      domain.ActionRefund,
			Status:      domain.JobStatusPending,
			MaxAttempts: 3,
			// Claim sorts by next_retry_at (regardless of the pending/processing
			// branch that matched) with a LIMIT, over test_payment, a shared
			// database that is never truncated (see the registry comment in
			// internal/testhelper/testhelper.go). 100 years is arbitrary -- it
			// only needs to predate every job that has ever accumulated so this
			// one is always claimed first and never crowded out of the LIMIT.
			NextRetryAt: time.Now().AddDate(-100, 0, 0),
		}
		require.NoError(t, repo.CreateJob(ctx, job))
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		// An expired locked_until makes only this job claimable, so it cannot steal one
		// from another test.
		_, err := testPool.Exec(
			ctx,
			`UPDATE payment_jobs SET last_error = $1, status = 'processing', locked_until = NOW() - INTERVAL '1 second' WHERE id = $2`,
			"some error",
			job.ID,
		)
		require.NoError(t, err)

		claimed, err := repo.Claim(ctx, 100, 30*time.Second)
		require.NoError(t, err)

		var claimedJob *domain.Job
		for i := range claimed {
			if claimed[i].ID == job.ID {
				claimedJob = &claimed[i]
			}
		}
		require.NotNil(t, claimedJob, "expected to reclaim the seeded job")
		assert.Equal(t, "some error", claimedJob.LastError)
	})
}

func TestPostgresRepository_MarkJobCompletedByPaymentID(t *testing.T) {
	t.Run("completes matching job", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		paymentID := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		job := &domain.Job{
			PaymentID:   paymentID,
			OrderID:     orderID,
			Action:      domain.ActionCharge,
			Status:      domain.JobStatusPending,
			MaxAttempts: 3,
			NextRetryAt: time.Now().Add(-time.Minute),
		}
		err := repo.CreateJob(ctx, job)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		err = repo.MarkJobCompletedByPaymentID(ctx, paymentID, domain.ActionCharge)
		require.NoError(t, err)

		claimed, err := repo.Claim(ctx, 100, 30*time.Second)
		require.NoError(t, err)
		for _, j := range claimed {
			assert.NotEqual(t, job.ID, j.ID, "completed job should not be claimable")
		}
	})

	t.Run("does not affect jobs with different action", func(t *testing.T) {
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		paymentID := seedPayment(t, orderID)
		repo := New(testPool)
		ctx := context.Background()

		job := &domain.Job{
			PaymentID:   paymentID,
			OrderID:     orderID,
			Action:      domain.ActionCharge,
			Status:      domain.JobStatusPending,
			MaxAttempts: 3,
			// Claim sorts oldest-next_retry_at-first with a LIMIT, over
			// test_payment, a shared database that is never truncated (see the
			// registry comment in internal/testhelper/testhelper.go). 100 years
			// is arbitrary -- it only needs to predate every job that has ever
			// accumulated so this one is always claimed first and never crowded
			// out of the LIMIT.
			NextRetryAt: time.Now().AddDate(-100, 0, 0),
		}
		err := repo.CreateJob(ctx, job)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		err = repo.MarkJobCompletedByPaymentID(ctx, paymentID, domain.ActionRefund)
		require.NoError(t, err)

		claimed, err := repo.Claim(ctx, 100, 30*time.Second)
		require.NoError(t, err)
		var found bool
		for _, j := range claimed {
			if j.ID == job.ID {
				found = true
			}
		}
		assert.True(t, found, "charge job should still be claimable after completing refund action")
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := New(testPool)

	t.Run("CreateJob", func(t *testing.T) {
		job := &domain.Job{
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    domain.ActionCharge,
			Status:    domain.JobStatusPending,
		}
		err := repo.CreateJob(cancelledCtx, job)
		assert.Error(t, err)
	})

	t.Run("Claim", func(t *testing.T) {
		_, err := repo.Claim(cancelledCtx, 10, 30*time.Second)
		assert.Error(t, err)
	})

	t.Run("UpdateJob", func(t *testing.T) {
		job := &domain.Job{ID: uuid.New(), Status: domain.JobStatusPending}
		err := repo.UpdateJob(cancelledCtx, job)
		assert.Error(t, err)
	})

	t.Run("CancelJobsByOrderID", func(t *testing.T) {
		err := repo.CancelJobsByOrderID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("MarkJobCompleted", func(t *testing.T) {
		err := repo.MarkJobCompleted(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("MarkJobCompletedByPaymentID", func(t *testing.T) {
		err := repo.MarkJobCompletedByPaymentID(cancelledCtx, uuid.New(), domain.ActionCharge)
		assert.Error(t, err)
	})

	t.Run("Prune", func(t *testing.T) {
		_, err := repo.Prune(cancelledCtx, 0, 100)
		assert.Error(t, err)
	})
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	return testhelper.SeedUser(t, testPool)
}

func seedOrder(t *testing.T, userID uuid.UUID) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO orders (id, user_id, status, subtotal_amount, discount_amount, total_amount, currency)
		 VALUES ($1, $2, 'awaiting_payment', 1000, 0, 1000, 'USD')`,
		id, userID,
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM orders WHERE id = $1`, id) })
	return id
}

// seedPayment inserts a payment row directly, purely as the foreign key
// payment_jobs.payment_id requires: jobs owns no operation on the payments
// table itself.
func seedPayment(t *testing.T, orderID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO payments (order_id, amount, currency, status, method)
		 VALUES ($1, $2, $3, $4, 'card')
		 RETURNING id`,
		orderID, money.New(1000, "USD").Amount, "USD", domain.StatusPending,
	).Scan(&id)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payments WHERE id = $1`, id) })
	return id
}
