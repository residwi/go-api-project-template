package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/residwi/go-api-project-template/internal/modules/notification"
	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

var _ notification.Repository = (*Repository)(nil)

type Repository struct {
	db database.DB
}

func New(db database.DB) *Repository {
	return &Repository{db: db}
}

func scanNotification(row pgx.CollectableRow) (domain.Notification, error) {
	var n domain.Notification
	err := row.Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt)
	return n, err
}

func (r *Repository) Create(ctx context.Context, n *domain.Notification) error {
	db := database.PrimaryDB(ctx, r.db)

	err := db.QueryRow(ctx,
		`INSERT INTO notifications (user_id, title, body, is_read)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at`,
		n.UserID, n.Title, n.Body, n.IsRead,
	).Scan(&n.ID, &n.CreatedAt)
	if err != nil {
		return fmt.Errorf("creating notification: %w", err)
	}
	return nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (domain.Notification, error) {
	db := database.PrimaryDB(ctx, r.db)

	var n domain.Notification
	err := db.QueryRow(ctx,
		`SELECT id, user_id, title, body, is_read, created_at FROM notifications WHERE id = $1`, id,
	).Scan(&n.ID, &n.UserID, &n.Title, &n.Body, &n.IsRead, &n.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Notification{}, errs.ErrNotFound
	}
	if err != nil {
		return domain.Notification{}, fmt.Errorf("getting notification: %w", err)
	}
	return n, nil
}

func (r *Repository) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Notification, error) {
	db := database.PrimaryDB(ctx, r.db)

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

	queryStr := fmt.Sprintf(
		`SELECT id, user_id, title, body, is_read, created_at
		FROM notifications WHERE %s ORDER BY created_at DESC, id DESC LIMIT $%d`,
		where, argIdx,
	)
	args = append(args, cursor.Limit+1)

	rows, err := db.Query(ctx, queryStr, args...)
	if err != nil {
		return nil, fmt.Errorf("listing notifications: %w", err)
	}
	notifications, err := pgx.CollectRows(rows, scanNotification)
	if err != nil {
		return nil, fmt.Errorf("listing notifications: %w", err)
	}

	return notifications, nil
}

func (r *Repository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	db := database.PrimaryDB(ctx, r.db)
	var count int
	err := db.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE user_id = $1 AND is_read = false`, userID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting unread notifications: %w", err)
	}
	return count, nil
}

func (r *Repository) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	db := database.PrimaryDB(ctx, r.db)
	tag, err := db.Exec(ctx,
		`UPDATE notifications SET is_read = true WHERE id = $1 AND user_id = $2`, id, userID,
	)
	if err != nil {
		return fmt.Errorf("marking notification read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errs.ErrNotFound
	}
	return nil
}

func (r *Repository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	db := database.PrimaryDB(ctx, r.db)
	_, err := db.Exec(ctx,
		`UPDATE notifications SET is_read = true WHERE user_id = $1 AND is_read = false`, userID,
	)
	if err != nil {
		return fmt.Errorf("marking all notifications read: %w", err)
	}
	return nil
}
