package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func scanNotification(row pgx.CollectableRow) (Notification, error) {
	var n Notification
	err := row.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt)
	return n, err
}

func scanJob(row pgx.CollectableRow) (Job, error) {
	var j Job
	var lastError *string
	if err := row.Scan(&j.ID, &j.UserID, &j.Type, &j.Title, &j.Body, &j.Data,
		&j.Status, &j.Attempts, &j.MaxAttempts, &lastError, &j.CreatedAt); err != nil {
		return Job{}, err
	}
	if lastError != nil {
		j.LastError = *lastError
	}
	return j, nil
}

type Repository interface {
	Create(ctx context.Context, n *Notification) error
	ListByUser(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Notification, error)
	MarkRead(ctx context.Context, userID, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	CreateJob(ctx context.Context, job *Job) error
	Claim(ctx context.Context, batchSize int, lease time.Duration) ([]Job, error)
	UpdateJob(ctx context.Context, job *Job) error
	Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, n *Notification) error {
	db := database.DB(ctx, r.pool)
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

func (r *PostgresRepository) ListByUser(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Notification, error) {
	db := database.DB(ctx, r.pool)

	args := []any{userID}
	where := "user_id = $1"
	argIdx := 2

	if cursor.Cursor != "" {
		var err error
		where, args, argIdx, err = database.KeysetCursor(where, args, argIdx, "created_at, id", cursor.Cursor)
		if err != nil {
			return nil, err
		}
	}

	query := fmt.Sprintf(
		`SELECT id, user_id, type, title, body, is_read, created_at
		FROM notifications WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, cursor.Limit+1)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("listing notifications: %w", err)
	}
	notifications, err := pgx.CollectRows(rows, scanNotification)
	if err != nil {
		return nil, fmt.Errorf("listing notifications: %w", err)
	}

	return notifications, nil
}

func (r *PostgresRepository) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	// Scope by user_id so a user can only mark their own notifications read (IDOR).
	tag, err := db.Exec(ctx,
		`UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("marking notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE notifications SET is_read = true WHERE user_id = $1 AND is_read = false`, userID,
	)
	if err != nil {
		return fmt.Errorf("marking all notifications read: %w", err)
	}
	return nil
}

func (r *PostgresRepository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	db := database.DB(ctx, r.pool)
	var count int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting unread notifications: %w", err)
	}
	return count, nil
}

func (r *PostgresRepository) CreateJob(ctx context.Context, job *Job) error {
	db := database.DB(ctx, r.pool)
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

func (r *PostgresRepository) Claim(ctx context.Context, batchSize int, lease time.Duration) ([]Job, error) {
	db := database.DB(ctx, r.pool)

	// Claim pending jobs AND reclaim 'processing' jobs whose lease has expired
	// (their worker died mid-processing), setting a fresh lease on each claim so
	// nothing can stay stuck in 'processing' indefinitely.
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
	jobs, err := pgx.CollectRows(rows, scanJob)
	if err != nil {
		return nil, fmt.Errorf("claiming pending jobs: %w", err)
	}

	return jobs, nil
}

func (r *PostgresRepository) UpdateJob(ctx context.Context, job *Job) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE notification_jobs SET status = $1, attempts = $2, last_error = $3
		WHERE id = $4`,
		job.Status, job.Attempts, job.LastError, job.ID,
	)
	if err != nil {
		return fmt.Errorf("updating notification job: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	db := database.DB(ctx, r.pool)

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

var _ Repository = (*PostgresRepository)(nil)
