package notification

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Repository interface {
	Create(ctx context.Context, n *Notification) error
	ListByUser(ctx context.Context, userID uuid.UUID, cursor core.CursorPage) ([]Notification, error)
	MarkRead(ctx context.Context, id uuid.UUID) error
	MarkAllRead(ctx context.Context, userID uuid.UUID) error
	CountUnread(ctx context.Context, userID uuid.UUID) (int, error)
	CreateJob(ctx context.Context, job *Job) error
	ClaimPendingJobs(ctx context.Context, batchSize int) ([]Job, error)
	UpdateJob(ctx context.Context, job *Job) error
	DeleteOldCompletedJobs(ctx context.Context, olderThan time.Duration, limit int) (int, error)
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
		cursorTime, cursorID, err := core.DecodeCursor(cursor.Cursor)
		if err != nil {
			return nil, fmt.Errorf("%w: invalid cursor", core.ErrBadRequest)
		}
		where += fmt.Sprintf(" AND (created_at, id) < ($%d, $%d)", argIdx, argIdx+1)
		args = append(args, cursorTime, cursorID)
		argIdx += 2
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
	defer rows.Close()

	var notifications []Notification
	for rows.Next() {
		var n Notification
		if err := rows.Scan(&n.ID, &n.UserID, &n.Type, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning notification: %w", err)
		}
		notifications = append(notifications, n)
	}

	return notifications, nil
}

func (r *PostgresRepository) MarkRead(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE notifications SET is_read = true WHERE id = $1 AND is_read = false`, id,
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

func (r *PostgresRepository) ClaimPendingJobs(ctx context.Context, batchSize int) ([]Job, error) {
	db := database.DB(ctx, r.pool)

	rows, err := db.Query(ctx,
		`WITH picked AS (
			SELECT id
			FROM notification_jobs
			WHERE status = 'pending' AND attempts < max_attempts
			ORDER BY created_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE notification_jobs j
		SET status = 'processing'
		FROM picked
		WHERE j.id = picked.id
		RETURNING j.id, j.user_id, j.type, j.title, j.body, j.data, j.status,
		          j.attempts, j.max_attempts, j.last_error, j.created_at`,
		batchSize,
	)
	if err != nil {
		return nil, fmt.Errorf("claiming pending jobs: %w", err)
	}
	defer rows.Close()

	var jobs []Job
	for rows.Next() {
		var j Job
		var lastError *string
		if err := rows.Scan(&j.ID, &j.UserID, &j.Type, &j.Title, &j.Body, &j.Data,
			&j.Status, &j.Attempts, &j.MaxAttempts, &lastError, &j.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning notification job: %w", err)
		}
		if lastError != nil {
			j.LastError = *lastError
		}
		jobs = append(jobs, j)
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

func (r *PostgresRepository) DeleteOldCompletedJobs(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	db := database.DB(ctx, r.pool)

	tag, err := db.Exec(ctx,
		`DELETE FROM notification_jobs
		WHERE id IN (
			SELECT id FROM notification_jobs
			WHERE status = 'completed' AND created_at < NOW() - $1::interval
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
