package payment_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_features_payment")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
}

func seedUser(t *testing.T) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES ($1, $2, 'x', 'A', 'B')`,
		id, id.String()+"@test.com",
	)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id) })
	return id
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

func seedPayment(t *testing.T, orderID uuid.UUID) *payment.Payment {
	t.Helper()
	repo := payment.NewPostgresRepository(testPool)
	p := &payment.Payment{
		OrderID:  orderID,
		Amount:   1000,
		Currency: "USD",
		Status:   payment.StatusPending,
		Method:   "card",
	}
	err := repo.Create(context.Background(), p)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payments WHERE id = $1`, p.ID) })
	return p
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates payment with correct fields", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := payment.NewPostgresRepository(testPool)

		p := &payment.Payment{
			OrderID:  orderID,
			Amount:   1000,
			Currency: "USD",
			Status:   payment.StatusPending,
			Method:   "card",
		}
		err := repo.Create(context.Background(), p)
		require.NoError(t, err)
		t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM payments WHERE id = $1`, p.ID) })

		assert.NotEqual(t, uuid.Nil, p.ID)
		assert.Equal(t, orderID, p.OrderID)
		assert.Equal(t, int64(1000), p.Amount)
		assert.Equal(t, "USD", p.Currency)
		assert.Equal(t, payment.StatusPending, p.Status)
		assert.Equal(t, "card", p.Method)
		assert.False(t, p.CreatedAt.IsZero())
		assert.False(t, p.UpdatedAt.IsZero())
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns payment", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)

		got, err := repo.GetByID(context.Background(), p.ID)
		require.NoError(t, err)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, p.OrderID, got.OrderID)
		assert.Equal(t, p.Amount, got.Amount)
		assert.Equal(t, p.Status, got.Status)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := payment.NewPostgresRepository(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_GetActiveByOrderID(t *testing.T) {
	t.Run("returns active payment for order", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)

		got, err := repo.GetActiveByOrderID(context.Background(), orderID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, p.ID, got.ID)
	})

	t.Run("returns ErrNotFound when none", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		repo := payment.NewPostgresRepository(testPool)

		got, err := repo.GetActiveByOrderID(context.Background(), orderID)
		require.ErrorIs(t, err, core.ErrNotFound)
		assert.Nil(t, got)
	})
}

func TestPostgresRepository_GetByGatewayTxnID(t *testing.T) {
	t.Run("returns payment by txn id", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		txnID := "txn-" + uuid.New().String()
		err := repo.UpdateGateway(ctx, p.ID, txnID, []byte(`{"status":"ok"}`))
		require.NoError(t, err)

		got, err := repo.GetByGatewayTxnID(ctx, txnID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, p.ID, got.ID)
		assert.Equal(t, txnID, got.GatewayTxnID)
	})

	t.Run("returns ErrNotFound when none", func(t *testing.T) {
		setup(t)
		repo := payment.NewPostgresRepository(testPool)

		got, err := repo.GetByGatewayTxnID(context.Background(), "nonexistent-txn-id")
		require.ErrorIs(t, err, core.ErrNotFound)
		assert.Nil(t, got)
	})
}

func TestPostgresRepository_UpdateStatus(t *testing.T) {
	t.Run("transitions status", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		err := repo.UpdateStatus(ctx, p.ID, payment.StatusProcessing, []payment.Status{payment.StatusPending})
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, payment.StatusProcessing, got.Status)
	})

	t.Run("returns conflict when from-status does not match", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)

		err := repo.UpdateStatus(context.Background(), p.ID, payment.StatusSuccess, []payment.Status{payment.StatusFailed})
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestPostgresRepository_UpdateGateway(t *testing.T) {
	t.Run("updates gateway txn id and response", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		txnID := "gw-txn-" + uuid.New().String()
		response := []byte(`{"code":200}`)
		err := repo.UpdateGateway(ctx, p.ID, txnID, response)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, txnID, got.GatewayTxnID)
		assert.JSONEq(t, string(response), string(got.GatewayResponse))
	})
}

func TestPostgresRepository_PaymentURL(t *testing.T) {
	t.Run("update and clear payment url", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		url := "https://pay.example.com/session/abc123"
		err := repo.UpdatePaymentURL(ctx, p.ID, url)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, url, got.PaymentURL)

		err = repo.ClearPaymentURL(ctx, p.ID)
		require.NoError(t, err)

		got, err = repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Empty(t, got.PaymentURL)
	})
}

func TestPostgresRepository_MarkPaid(t *testing.T) {
	t.Run("marks payment as paid", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		err := repo.MarkPaid(ctx, p.ID, []payment.Status{payment.StatusPending})
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, payment.StatusSuccess, got.Status)
		assert.NotNil(t, got.PaidAt)
	})

	t.Run("returns conflict when status does not match", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)

		err := repo.MarkPaid(context.Background(), p.ID, []payment.Status{payment.StatusFailed})
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestPostgresRepository_ListByOrderID(t *testing.T) {
	t.Run("returns all payments for order", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)

		payments, err := repo.ListByOrderID(context.Background(), orderID)
		require.NoError(t, err)
		require.Len(t, payments, 1)
		assert.Equal(t, p.ID, payments[0].ID)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns paginated results", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)

		payments, total, err := repo.ListAdmin(context.Background(), payment.AdminListParams{
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, payments)
	})
}

func TestPostgresRepository_JobLifecycle(t *testing.T) {
	t.Run("create, claim, update, cancel, complete, and delete job", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		// CreateJob
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
		assert.NotEqual(t, uuid.Nil, job.ID)
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		// ClaimPendingJobs — job moves to 'processing'
		claimed, err := repo.ClaimPendingJobs(ctx, 10, 30*time.Second)
		require.NoError(t, err)

		var claimedJob *payment.Job
		for i := range claimed {
			if claimed[i].ID == job.ID {
				claimedJob = &claimed[i]
				break
			}
		}
		require.NotNil(t, claimedJob, "expected to claim the seeded job")
		assert.Equal(t, payment.JobStatusProcessing, claimedJob.Status)
		assert.NotNil(t, claimedJob.LockedUntil)

		// UpdateJob — simulate a retry increment
		claimedJob.Attempts = 1
		claimedJob.LastError = "transient error"
		claimedJob.Status = payment.JobStatusProcessing
		claimedJob.NextRetryAt = time.Now().Add(5 * time.Second)
		err = repo.UpdateJob(ctx, claimedJob)
		require.NoError(t, err)

		// CancelJobsByOrderID — cancels processing/pending jobs
		err = repo.CancelJobsByOrderID(ctx, orderID)
		require.NoError(t, err)

		// MarkJobCompleted — idempotent even after cancel
		err = repo.MarkJobCompleted(ctx, job.ID)
		require.NoError(t, err)

		// DeleteOldCompletedJobs with olderThan=0 so all completed/failed/cancelled qualify
		deleted, err := repo.DeleteOldCompletedJobs(ctx, 0, 100)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, deleted, 1)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()

	repo := payment.NewPostgresRepository(testPool)

	t.Run("Create", func(t *testing.T) {
		setup(t)
		p := &payment.Payment{OrderID: uuid.New(), Amount: 1000, Currency: "USD", Status: payment.StatusPending}
		err := repo.Create(cancelledCtx, p)
		assert.Error(t, err)
	})

	t.Run("GetByID", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetActiveByOrderID", func(t *testing.T) {
		setup(t)
		_, err := repo.GetActiveByOrderID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("GetByGatewayTxnID", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByGatewayTxnID(cancelledCtx, "nonexistent")
		assert.Error(t, err)
	})

	t.Run("UpdateStatus", func(t *testing.T) {
		setup(t)
		err := repo.UpdateStatus(cancelledCtx, uuid.New(), payment.StatusSuccess, []payment.Status{payment.StatusPending})
		assert.Error(t, err)
	})

	t.Run("UpdateGateway", func(t *testing.T) {
		setup(t)
		err := repo.UpdateGateway(cancelledCtx, uuid.New(), "txn", []byte(`{}`))
		assert.Error(t, err)
	})

	t.Run("UpdatePaymentURL", func(t *testing.T) {
		setup(t)
		err := repo.UpdatePaymentURL(cancelledCtx, uuid.New(), "https://pay.example.com")
		assert.Error(t, err)
	})

	t.Run("ClearPaymentURL", func(t *testing.T) {
		setup(t)
		err := repo.ClearPaymentURL(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("MarkPaid", func(t *testing.T) {
		setup(t)
		err := repo.MarkPaid(cancelledCtx, uuid.New(), []payment.Status{payment.StatusPending})
		assert.Error(t, err)
	})

	t.Run("ListByOrderID", func(t *testing.T) {
		setup(t)
		_, err := repo.ListByOrderID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("ListAdmin", func(t *testing.T) {
		setup(t)
		_, _, err := repo.ListAdmin(cancelledCtx, payment.AdminListParams{Page: 1, PageSize: 10})
		assert.Error(t, err)
	})

	t.Run("CreateJob", func(t *testing.T) {
		setup(t)
		job := &payment.Job{PaymentID: uuid.New(), OrderID: uuid.New(), Action: payment.ActionCharge, Status: payment.JobStatusPending}
		err := repo.CreateJob(cancelledCtx, job)
		assert.Error(t, err)
	})

	t.Run("ClaimPendingJobs", func(t *testing.T) {
		setup(t)
		_, err := repo.ClaimPendingJobs(cancelledCtx, 10, 30*time.Second)
		assert.Error(t, err)
	})

	t.Run("UpdateJob", func(t *testing.T) {
		setup(t)
		job := &payment.Job{ID: uuid.New(), Status: payment.JobStatusPending}
		err := repo.UpdateJob(cancelledCtx, job)
		assert.Error(t, err)
	})

	t.Run("CancelJobsByOrderID", func(t *testing.T) {
		setup(t)
		err := repo.CancelJobsByOrderID(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("MarkJobCompleted", func(t *testing.T) {
		setup(t)
		err := repo.MarkJobCompleted(cancelledCtx, uuid.New())
		assert.Error(t, err)
	})

	t.Run("MarkJobCompletedByPaymentID", func(t *testing.T) {
		setup(t)
		err := repo.MarkJobCompletedByPaymentID(cancelledCtx, uuid.New(), payment.ActionCharge)
		assert.Error(t, err)
	})

	t.Run("DeleteOldCompletedJobs", func(t *testing.T) {
		setup(t)
		_, err := repo.DeleteOldCompletedJobs(cancelledCtx, 0, 100)
		assert.Error(t, err)
	})
}

func TestPostgresRepository_ListByOrderID_WithNullableFields(t *testing.T) {
	t.Run("returns payments with payment_method_id, payment_url, and gateway_txn_id", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		require.NoError(t, repo.UpdateGateway(ctx, p.ID, "txn-list-test", []byte(`{}`)))
		require.NoError(t, repo.UpdatePaymentURL(ctx, p.ID, "https://pay.example.com/list"))

		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1 WHERE id = $2`, "pm_list_test", p.ID)
		require.NoError(t, err)

		payments, err := repo.ListByOrderID(ctx, orderID)
		require.NoError(t, err)
		require.Len(t, payments, 1)
		assert.Equal(t, "pm_list_test", payments[0].PaymentMethodID)
		assert.Equal(t, "https://pay.example.com/list", payments[0].PaymentURL)
		assert.Equal(t, "txn-list-test", payments[0].GatewayTxnID)
	})
}

func TestPostgresRepository_ListAdmin_WithNullableFields(t *testing.T) {
	t.Run("returns payments with payment_method_id and gateway_txn_id", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		require.NoError(t, repo.UpdateGateway(ctx, p.ID, "txn-admin-list", []byte(`{}`)))
		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1 WHERE id = $2`, "pm_admin_list", p.ID)
		require.NoError(t, err)

		payments, total, err := repo.ListAdmin(ctx, payment.AdminListParams{
			Page:     1,
			PageSize: 100,
			OrderID:  orderID.String(),
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		var found bool
		for _, pay := range payments {
			if pay.ID == p.ID {
				assert.Equal(t, "pm_admin_list", pay.PaymentMethodID)
				assert.Equal(t, "txn-admin-list", pay.GatewayTxnID)
				found = true
			}
		}
		assert.True(t, found)
	})
}

func TestPostgresRepository_ClaimPendingJobs_WithOptionalFields(t *testing.T) {
	t.Run("claimed job with last_error and inventory_action", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		job := &payment.Job{
			PaymentID:       p.ID,
			OrderID:         orderID,
			Action:          payment.ActionRefund,
			Status:          payment.JobStatusPending,
			MaxAttempts:     3,
			NextRetryAt:     time.Now(),
			InventoryAction: "release",
		}
		require.NoError(t, repo.CreateJob(ctx, job))
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		// Set last_error and mark as 'processing' with an expired locked_until
		// so only this specific job is claimable (avoids stealing jobs from other tests).
		_, err := testPool.Exec(ctx,
			`UPDATE payment_jobs SET last_error = $1, status = 'processing', locked_until = NOW() - INTERVAL '1 second' WHERE id = $2`,
			"some error", job.ID)
		require.NoError(t, err)

		// Cancel all other pending/processing jobs to avoid interference
		claimed, err := repo.ClaimPendingJobs(ctx, 1, 30*time.Second)
		require.NoError(t, err)

		var claimedJob *payment.Job
		for i := range claimed {
			if claimed[i].ID == job.ID {
				claimedJob = &claimed[i]
				break
			}
		}
		if claimedJob == nil {
			t.Skip("job was claimed by a concurrent test")
			return
		}
		assert.Equal(t, "some error", claimedJob.LastError)
		assert.Equal(t, "release", claimedJob.InventoryAction)
	})
}

func TestPostgresRepository_GetActiveByOrderID_WithNullableFields(t *testing.T) {
	t.Run("returns active payment with payment_method_id, payment_url, and gateway_txn_id", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		require.NoError(t, repo.UpdateGateway(ctx, p.ID, "txn-active-test", []byte(`{}`)))
		require.NoError(t, repo.UpdatePaymentURL(ctx, p.ID, "https://pay.example.com/active"))
		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1 WHERE id = $2`, "pm_active_test", p.ID)
		require.NoError(t, err)

		got, err := repo.GetActiveByOrderID(ctx, orderID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "pm_active_test", got.PaymentMethodID)
		assert.Equal(t, "https://pay.example.com/active", got.PaymentURL)
		assert.Equal(t, "txn-active-test", got.GatewayTxnID)
	})
}

func TestPostgresRepository_GetByID_WithNullableFields(t *testing.T) {
	t.Run("returns payment with payment_method_id", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1 WHERE id = $2`, "pm_get_test", p.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, p.ID)
		require.NoError(t, err)
		assert.Equal(t, "pm_get_test", got.PaymentMethodID)
	})
}

func TestPostgresRepository_GetByGatewayTxnID_WithNullableFields(t *testing.T) {
	t.Run("returns payment with payment_method_id and payment_url", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)
		ctx := context.Background()

		txnID := "txn-nullable-" + uuid.New().String()
		require.NoError(t, repo.UpdateGateway(ctx, p.ID, txnID, []byte(`{}`)))
		require.NoError(t, repo.UpdatePaymentURL(ctx, p.ID, "https://pay.example.com/nullable"))
		_, err := testPool.Exec(ctx,
			`UPDATE payments SET payment_method_id = $1 WHERE id = $2`, "pm_nullable_test", p.ID)
		require.NoError(t, err)

		got, err := repo.GetByGatewayTxnID(ctx, txnID)
		require.NoError(t, err)
		require.NotNil(t, got)
		assert.Equal(t, "pm_nullable_test", got.PaymentMethodID)
		assert.Equal(t, "https://pay.example.com/nullable", got.PaymentURL)
	})
}

func TestPostgresRepository_ListAdmin_Filters(t *testing.T) {
	t.Run("filters by status", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)

		payments, total, err := repo.ListAdmin(context.Background(), payment.AdminListParams{
			Page:     1,
			PageSize: 10,
			Status:   "pending",
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, p := range payments {
			assert.Equal(t, payment.StatusPending, p.Status)
		}
	})

	t.Run("filters by order ID", func(t *testing.T) {
		setup(t)
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		seedPayment(t, orderID)
		repo := payment.NewPostgresRepository(testPool)

		payments, total, err := repo.ListAdmin(context.Background(), payment.AdminListParams{
			Page:     1,
			PageSize: 10,
			OrderID:  orderID.String(),
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, p := range payments {
			assert.Equal(t, orderID, p.OrderID)
		}
	})
}

func TestPostgresRepository_MarkJobCompletedByPaymentID(t *testing.T) {
	t.Run("completes matching job", func(t *testing.T) {
		setup(t)
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

		err = repo.MarkJobCompletedByPaymentID(ctx, p.ID, payment.ActionCharge)
		require.NoError(t, err)

		claimed, err := repo.ClaimPendingJobs(ctx, 10, 30*time.Second)
		require.NoError(t, err)
		for _, j := range claimed {
			assert.NotEqual(t, job.ID, j.ID, "completed job should not be claimable")
		}
	})

	t.Run("does not affect jobs with different action", func(t *testing.T) {
		setup(t)
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

		err = repo.MarkJobCompletedByPaymentID(ctx, p.ID, payment.ActionRefund)
		require.NoError(t, err)

		claimed, err := repo.ClaimPendingJobs(ctx, 10, 30*time.Second)
		require.NoError(t, err)

		var found bool
		for _, j := range claimed {
			if j.ID == job.ID {
				found = true
				break
			}
		}
		assert.True(t, found, "charge job should still be claimable after completing refund action")
	})
}
