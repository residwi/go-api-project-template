package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/modules/notification/jobs"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ jobs.Repository = (*Repository)(nil)

func scanJob(row pgx.CollectableRow) (domain.Job, error) {
	var j domain.Job
	var lastError *string
	if err := row.Scan(&j.ID, &j.UserID, &j.Type, &j.Title, &j.Body, &j.Data,
		&j.Status, &j.Attempts, &j.MaxAttempts, &lastError, &j.CreatedAt); err != nil {
		return domain.Job{}, err
	}
	if lastError != nil {
		j.LastError = *lastError
	}
	return j, nil
}

type Repository struct {
	db database.DB
}

func New(db database.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, n *domain.Notification) error {
	db := database.PrimaryDB(ctx, r.db)
	err := db.QueryRow(ctx,
		`INSERT INTO notifications (user_id, type, title, body, is_read, data)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id, created_at`,
		n.UserID, n.Type, n.Title, n.Body, n.IsRead, n.Data,
	).Scan(&n.ID, &n.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating notification: %w", err)
	}
	return nil
}

func (r *Repository) CreateJob(ctx context.Context, job *domain.Job) error {
	db := database.PrimaryDB(ctx, r.db)
	err := db.QueryRow(ctx,
		`INSERT INTO notification_jobs (user_id, type, title, body, data, status, attempts, max_attempts)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, created_at`,
		job.UserID, job.Type, job.Title, job.Body, job.Data, job.Status, job.Attempts, job.MaxAttempts,
	).Scan(&job.ID, &job.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating notification job: %w", err)
	}
	return nil
}

func (r *Repository) Claim(ctx context.Context, batchSize int, lease time.Duration) ([]domain.Job, error) {
	db := database.PrimaryDB(ctx, r.db)

	rows, err := db.Query(ctx,
		`WITH picked AS (
			SELECT id
			FROM notification_jobs
			WHERE (status = 'pending' OR (status = 'processing' AND locked_until <= NOW()))
			  AND attempts < max_attempts
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notification_jobs j
		SET status = 'processing', locked_until = NOW() + $2::interval
		FROM picked
		WHERE j.id = picked.id
		RETURNING j.id, j.user_id, j.type, j.title, j.body, j.data, j.status,
		          j.attempts, j.max_attempts, j.last_error, j.created_at`,
		batchSize, lease.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("claiming pending jobs: %w", err)
	}
	jobList, err := pgx.CollectRows(rows, scanJob)
	if err != nil {
		return nil, fmt.Errorf("claiming pending jobs: %w", err)
	}

	return jobList, nil
}

func (r *Repository) UpdateJob(ctx context.Context, job *domain.Job) error {
	db := database.PrimaryDB(ctx, r.db)
	tag, err := db.Exec(ctx,
		`UPDATE notification_jobs SET status = $1, attempts = $2, last_error = $3
		WHERE id = $4`,
		job.Status, job.Attempts, job.LastError, job.ID,
	)
	if err != nil {
		return fmt.Errorf("updating notification job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

func (r *Repository) CreateAndComplete(ctx context.Context, n *domain.Notification, job *domain.Job) error {
	return database.WithTx(ctx, r.db.Primary, func(txCtx context.Context) error {
		if err := r.Create(txCtx, n); err != nil {
			return err
		}
		return r.UpdateJob(txCtx, job)
	})
}

func (r *Repository) Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	db := database.PrimaryDB(ctx, r.db)

	tag, err := db.Exec(ctx,
		`DELETE FROM notification_jobs
		WHERE id IN (
			SELECT id FROM notification_jobs
			WHERE status IN ('completed', 'failed') AND created_at < NOW() - $1::interval
			LIMIT $2
		)`,
		olderThan.String(), limit,
	)
	if err != nil {
		return 0, fmt.Errorf("deleting old completed jobs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
