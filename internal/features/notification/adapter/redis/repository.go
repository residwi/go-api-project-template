package redis

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/features/notification"
	"github.com/residwi/go-api-project-template/internal/features/notification/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

const (
	unreadTTL = 5 * time.Minute

	adjustScript = `
if redis.call('EXISTS', KEYS[1]) == 0 then return -1 end
local v = redis.call('INCRBY', KEYS[1], ARGV[1])
if v < 0 then
	redis.call('SET', KEYS[1], 0, 'KEEPTTL')
	return 0
end
return v
`
)

type Repository struct {
	repo   notification.Repository
	rdb    *goredis.Client
	logger *slog.Logger
}

var _ notification.Repository = (*Repository)(nil)

func New(repo notification.Repository, rdb *goredis.Client, logger *slog.Logger) *Repository {
	return &Repository{
		repo:   repo,
		rdb:    rdb,
		logger: logger,
	}
}

func (r *Repository) Create(ctx context.Context, n *domain.Notification) error {
	if err := r.repo.Create(ctx, n); err != nil {
		return err
	}

	r.adjust(ctx, unreadKey(n.UserID), 1)

	return nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (domain.Notification, error) {
	return r.repo.Get(ctx, id)
}

func (r *Repository) ListByUser(
	ctx context.Context,
	userID uuid.UUID,
	cursor paging.CursorPage,
) ([]domain.Notification, error) {
	return r.repo.ListByUser(ctx, userID, cursor)
}

func (r *Repository) CountUnread(ctx context.Context, userID uuid.UUID) (int, error) {
	key := unreadKey(userID)

	count, err := r.rdb.Get(ctx, key).Int()
	if err == nil {
		return count, nil
	}

	if !errors.Is(err, goredis.Nil) {
		r.logger.WarnContext(ctx, "cache read failed, falling back to loader",
			slog.String("key", key), slog.String("error", err.Error()))
	}

	count, err = r.repo.CountUnread(ctx, userID)
	if err != nil {
		return 0, err
	}

	r.store(ctx, key, count)

	return count, nil
}

func (r *Repository) MarkRead(ctx context.Context, userID, id uuid.UUID) error {
	if err := r.repo.MarkRead(ctx, userID, id); err != nil {
		return err
	}

	r.adjust(ctx, unreadKey(userID), -1)

	return nil
}

func (r *Repository) MarkAllRead(ctx context.Context, userID uuid.UUID) error {
	if err := r.repo.MarkAllRead(ctx, userID); err != nil {
		return err
	}

	r.store(ctx, unreadKey(userID), 0)

	return nil
}

func (r *Repository) adjust(ctx context.Context, key string, delta int64) {
	writeCtx := context.WithoutCancel(ctx)

	if err := r.rdb.Eval(writeCtx, adjustScript, []string{key}, delta).Err(); err != nil {
		r.logger.WarnContext(writeCtx, "unread counter update failed",
			slog.String("key", key), slog.String("error", err.Error()))
	}
}

func (r *Repository) store(ctx context.Context, key string, count int) {
	writeCtx := context.WithoutCancel(ctx)

	if err := r.rdb.Set(writeCtx, key, count, unreadTTL).Err(); err != nil {
		r.logger.WarnContext(writeCtx, "unread counter write failed",
			slog.String("key", key), slog.String("error", err.Error()))
	}
}

func unreadKey(userID uuid.UUID) string { return "notification:unread:" + userID.String() }
