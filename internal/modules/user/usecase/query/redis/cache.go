package redis

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"
	goredis "github.com/redis/go-redis/v9"

	"github.com/residwi/go-api-project-template/internal/modules/user/usecase/query"
)

type Cache struct{ rdb *goredis.Client }

func New(rdb *goredis.Client) *Cache { return &Cache{rdb: rdb} }

func (c *Cache) Get(ctx context.Context, userID uuid.UUID) (query.StatusSnapshot, bool, error) {
	fields, err := c.rdb.HGetAll(ctx, statusKey(userID)).Result()
	if err != nil {
		return query.StatusSnapshot{}, false, err
	}
	if len(fields) == 0 {
		return query.StatusSnapshot{}, false, nil
	}
	activeField, ok := fields["active"]
	if !ok {
		return query.StatusSnapshot{}, false, nil
	}

	tokenVersion, err := strconv.Atoi(fields["token_version"])
	if err != nil {
		return query.StatusSnapshot{}, false, nil //nolint:nilerr // malformed cache entry is a deliberate miss, not a propagated error
	}

	return query.StatusSnapshot{Active: activeField == "1", TokenVersion: tokenVersion}, true, nil
}

func (c *Cache) Put(ctx context.Context, userID uuid.UUID, snap query.StatusSnapshot, ttl time.Duration) error {
	active := "0"
	if snap.Active {
		active = "1"
	}
	opts := &goredis.HSetEXOptions{
		ExpirationType: goredis.HSetEXExpirationEX,
		ExpirationVal:  int64(ttl.Seconds()),
	}
	return c.rdb.HSetEXWithArgs(ctx, statusKey(userID), opts,
		"active", active,
		"token_version", strconv.Itoa(snap.TokenVersion),
	).Err()
}

func (c *Cache) Invalidate(ctx context.Context, userID uuid.UUID) error {
	return c.rdb.Del(ctx, statusKey(userID)).Err()
}

func statusKey(userID uuid.UUID) string { return "user:status:" + userID.String() }

var _ query.StatusCache = (*Cache)(nil)
