package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// Repository backs payment's JobRepository port. No compile-time assertion
// against it here: payment's own New constructs this Repository directly
// from Deps.Pool (see payment/jobs.go), so an import running the other way
// -- this package back into payment, just to write
// "var _ payment.JobRepository = (*Repository)(nil)" -- would cycle.
// Structural typing still catches a mismatch at that construction call site.
type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

// defaultJobMaxAttempts mirrors the payment_jobs.max_attempts column default.
const defaultJobMaxAttempts = 3

func (r *Repository) CreateJob(ctx context.Context, job *domain.Job) error {
	db := database.DB(ctx, r.pool)

	if job.MaxAttempts <= 0 {
		job.MaxAttempts = defaultJobMaxAttempts
	}

	err := db.QueryRow(ctx,
		`INSERT INTO payment_jobs (payment_id, order_id, action, status, max_attempts, locked_until, next_retry_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		job.PaymentID, job.OrderID, job.Action, job.Status, job.MaxAttempts,
		job.LockedUntil, job.NextRetryAt,
	).Scan(&job.ID, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating payment job: %w", err)
	}
	return nil
}

func (r *Repository) Claim(ctx context.Context, batchSize int, leaseDuration time.Duration) ([]domain.Job, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`WITH picked AS (
			SELECT id
			FROM payment_jobs
			WHERE (
				(status = 'pending' AND next_retry_at <= NOW())
				OR (status = 'processing' AND locked_until <= NOW())
			)
			AND attempts < max_attempts
			ORDER BY next_retry_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE payment_jobs j
		SET status = 'processing',
		    locked_until = NOW() + $2::interval
		FROM picked
		WHERE j.id = picked.id
		RETURNING j.id, j.payment_id, j.order_id, j.action, j.status, j.attempts,
		          j.max_attempts, j.last_error, j.locked_until, j.next_retry_at,
		          j.created_at, j.updated_at`,
		batchSize, leaseDuration.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("claiming pending jobs: %w", err)
	}

	claimed, err := pgx.CollectRows(rows, scanJob)
	if err != nil {
		return nil, fmt.Errorf("iterating payment jobs: %w", err)
	}
	return claimed, nil
}

func scanJob(row pgx.CollectableRow) (domain.Job, error) {
	var j domain.Job
	var lastError *string
	if err := row.Scan(&j.ID, &j.PaymentID, &j.OrderID, &j.Action, &j.Status,
		&j.Attempts, &j.MaxAttempts, &lastError, &j.LockedUntil, &j.NextRetryAt,
		&j.CreatedAt, &j.UpdatedAt); err != nil {
		return domain.Job{}, fmt.Errorf("scanning payment job: %w", err)
	}
	if lastError != nil {
		j.LastError = *lastError
	}
	return j, nil
}

func (r *Repository) UpdateJob(ctx context.Context, job *domain.Job) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payment_jobs SET status = $1, attempts = $2, last_error = $3,
		 locked_until = $4, next_retry_at = $5
		 WHERE id = $6`,
		job.Status, job.Attempts, nilIfEmpty(job.LastError),
		job.LockedUntil, job.NextRetryAt, job.ID,
	)
	if err != nil {
		return fmt.Errorf("updating payment job: %w", err)
	}
	return nil
}

func (r *Repository) CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payment_jobs SET status = 'cancelled' WHERE order_id = $1 AND status IN ('pending', 'processing')`,
		orderID,
	)
	if err != nil {
		return fmt.Errorf("cancelling jobs by order: %w", err)
	}
	return nil
}

func (r *Repository) MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payment_jobs SET status = 'completed', locked_until = NULL WHERE id = $1`,
		jobID,
	)
	if err != nil {
		return fmt.Errorf("marking job completed: %w", err)
	}
	return nil
}

func (r *Repository) MarkJobCompletedByPaymentID(
	ctx context.Context,
	paymentID uuid.UUID,
	action domain.JobAction,
) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payment_jobs SET status = 'completed', locked_until = NULL
		 WHERE payment_id = $1 AND action = $2 AND status IN ('pending', 'processing')`,
		paymentID, action,
	)
	if err != nil {
		return fmt.Errorf("marking job completed by payment: %w", err)
	}
	return nil
}

func (r *Repository) Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`DELETE FROM payment_jobs WHERE id IN (
			SELECT id FROM payment_jobs
			WHERE status IN ('completed', 'failed', 'cancelled')
			AND updated_at < NOW() - $1::interval
			LIMIT $2
		)`,
		olderThan.String(), limit,
	)
	if err != nil {
		return 0, fmt.Errorf("deleting old completed jobs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
