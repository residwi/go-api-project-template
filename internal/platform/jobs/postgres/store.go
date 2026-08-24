package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/jobs"
)

var _ jobs.Store = (*Store)(nil)

type Store struct {
	db database.DB
}

func New(db database.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Insert(ctx context.Context, r jobs.Record) error {
	db := database.PrimaryDB(ctx, s.db)

	_, err := db.Exec(ctx,
		`INSERT INTO job_queue (queue, kind, payload, dedup_key, group_key, status, max_attempts, run_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), NULLIF($5, ''), $6, $7, $8)`,
		r.Queue, r.Kind, r.Payload, r.DedupKey, r.GroupKey, r.Status, r.MaxAttempts, r.RunAt,
	)
	if err != nil {
		return fmt.Errorf("inserting job: %w", err)
	}
	return nil
}

func (s *Store) Claim(ctx context.Context, queue string, batch int, lease time.Duration) ([]jobs.Record, error) {
	db := database.PrimaryDB(ctx, s.db)

	rows, err := db.Query(ctx,
		`UPDATE job_queue SET status = 'processing', locked_until = NOW() + $1::interval, updated_at = NOW()
		WHERE id IN (
			SELECT id FROM job_queue
			WHERE queue = $2 AND status = 'pending' AND run_at <= NOW()
			ORDER BY run_at
			LIMIT $3
			FOR UPDATE SKIP LOCKED
		)
		RETURNING id, queue, kind, payload, COALESCE(dedup_key, ''), COALESCE(group_key, ''),
		          status, attempts, max_attempts, last_error, locked_until, run_at`,
		lease.String(), queue, batch,
	)
	if err != nil {
		return nil, fmt.Errorf("claiming jobs: %w", err)
	}

	claimed, err := pgx.CollectRows(rows, scanRecord)
	if err != nil {
		return nil, fmt.Errorf("collecting claimed jobs: %w", err)
	}
	return claimed, nil
}

func (s *Store) Complete(ctx context.Context, id uuid.UUID) error {
	return s.settle(ctx, `UPDATE job_queue SET status = 'completed', locked_until = NULL,
		updated_at = NOW() WHERE id = $1`, id)
}

func (s *Store) Retry(ctx context.Context, id uuid.UUID, attempts int, lastErr string, runAt time.Time) error {
	return s.settle(ctx, `UPDATE job_queue SET status = 'pending', attempts = $2, last_error = $3,
		run_at = $4, locked_until = NULL, updated_at = NOW() WHERE id = $1`, id, attempts, lastErr, runAt)
}

func (s *Store) Bury(ctx context.Context, id uuid.UUID, attempts int, lastErr string) error {
	return s.settle(ctx, `UPDATE job_queue SET status = 'dead', attempts = $2, last_error = $3,
		locked_until = NULL, updated_at = NOW() WHERE id = $1`, id, attempts, lastErr)
}

func (s *Store) Cancel(ctx context.Context, id uuid.UUID, lastErr string) error {
	return s.settle(ctx, `UPDATE job_queue SET status = 'cancelled', last_error = $2,
		locked_until = NULL, updated_at = NOW() WHERE id = $1`, id, lastErr)
}

func (s *Store) CancelByDedupKey(ctx context.Context, dedupKey string) (int, error) {
	return s.cancelWhere(ctx, `UPDATE job_queue SET status = 'cancelled', locked_until = NULL,
		updated_at = NOW() WHERE dedup_key = $1 AND status = 'pending'`, dedupKey)
}

func (s *Store) CancelByGroupKey(ctx context.Context, groupKey string) (int, error) {
	return s.cancelWhere(ctx, `UPDATE job_queue SET status = 'cancelled', locked_until = NULL,
		updated_at = NOW() WHERE group_key = $1 AND status = 'pending'`, groupKey)
}

func (s *Store) Prune(ctx context.Context, queue string, age time.Duration, limit int) (int, error) {
	db := database.PrimaryDB(ctx, s.db)

	tag, err := db.Exec(ctx,
		`DELETE FROM job_queue WHERE id IN (
			SELECT id FROM job_queue
			WHERE queue = $1
			  AND status IN ('completed', 'cancelled')
			  AND updated_at < NOW() - $2::interval
			LIMIT $3
		)`,
		queue, age.String(), limit,
	)
	if err != nil {
		return 0, fmt.Errorf("pruning jobs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func (s *Store) settle(ctx context.Context, query string, args ...any) error {
	db := database.PrimaryDB(ctx, s.db)

	if _, err := db.Exec(ctx, query, args...); err != nil {
		return fmt.Errorf("settling job: %w", err)
	}
	return nil
}

func (s *Store) cancelWhere(ctx context.Context, query, key string) (int, error) {
	db := database.PrimaryDB(ctx, s.db)

	tag, err := db.Exec(ctx, query, key)
	if err != nil {
		return 0, fmt.Errorf("cancelling jobs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func scanRecord(row pgx.CollectableRow) (jobs.Record, error) {
	var r jobs.Record
	var lastError *string
	if err := row.Scan(&r.ID, &r.Queue, &r.Kind, &r.Payload, &r.DedupKey, &r.GroupKey,
		&r.Status, &r.Attempts, &r.MaxAttempts, &lastError, &r.LockedUntil, &r.RunAt); err != nil {
		return jobs.Record{}, err
	}
	if lastError != nil {
		r.LastError = *lastError
	}
	return r, nil
}
