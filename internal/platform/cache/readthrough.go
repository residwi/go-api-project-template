package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

type Options struct {
	TTL         time.Duration
	AbsentTTL   time.Duration
	Deviation   float64
	LoadTimeout time.Duration
}

type ReadThrough struct {
	rdb    *redis.Client
	logger *slog.Logger
	flight singleflight.Group
}

func NewReadThrough(rdb *redis.Client, logger *slog.Logger) *ReadThrough {
	return &ReadThrough{rdb: rdb, logger: logger}
}

func (r *ReadThrough) Take[T any](
	ctx context.Context,
	opts Options,
	key string,
	load func(context.Context) (T, error),
) (T, error) {
	opts.mustBeValid()

	var zero T

	ch := r.flight.DoChan(key, func() (any, error) {
		return r.fill(ctx, opts, key, load)
	})

	select {
	case res := <-ch:
		if res.Err != nil {
			return zero, res.Err
		}

		val, ok := res.Val.(T)
		if !ok {
			return zero, fmt.Errorf("cache: mismatched type for key %q", key)
		}

		return val, nil
	case <-ctx.Done():
		return zero, ctx.Err()
	}
}

const (
	absentPlaceholder = "\x00absent"
	writeTimeout      = 500 * time.Millisecond
)

func (r *ReadThrough) fill[T any](
	ctx context.Context,
	opts Options,
	key string,
	load func(context.Context) (T, error),
) (_ any, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			err = fmt.Errorf("cache load panicked: %s", fmt.Sprint(rec))
		}
	}()

	var zero T

	shared := context.WithoutCancel(ctx)

	raw, getErr := r.rdb.Get(shared, key).Result()
	if getErr == nil {
		if raw == absentPlaceholder {
			return zero, errs.ErrNotFound
		}

		var val T
		unmarshalErr := json.Unmarshal([]byte(raw), &val)
		if unmarshalErr == nil {
			return val, nil
		}

		r.logger.WarnContext(shared, "corrupt cache entry, dropping",
			slog.String("key", key), slog.String("error", unmarshalErr.Error()))

		r.del(shared, key)
	} else if !errors.Is(getErr, redis.Nil) {
		r.logger.WarnContext(shared, "cache read failed, falling back to loader",
			slog.String("key", key), slog.String("error", getErr.Error()))
	}

	loadCtx, cancelLoad := context.WithTimeout(shared, opts.LoadTimeout)
	defer cancelLoad()

	val, err := load(loadCtx)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			r.write(shared, key, absentPlaceholder, jitter(opts.AbsentTTL, opts.Deviation), true)
		}

		return zero, err
	}

	data, marshalErr := json.Marshal(val)
	if marshalErr != nil {
		r.logger.WarnContext(
			shared,
			"cache write failed",
			slog.String("key", key),
			slog.String("error", marshalErr.Error()),
		)

		return val, nil
	}

	r.write(shared, key, data, jitter(opts.TTL, opts.Deviation), false)

	return val, nil
}

func (r *ReadThrough) write(ctx context.Context, key string, data any, ttl time.Duration, absent bool) {
	writeCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	var err error
	if absent {
		err = r.rdb.SetNX(writeCtx, key, data, ttl).Err()
	} else {
		err = r.rdb.Set(writeCtx, key, data, ttl).Err()
	}

	if err != nil {
		r.logger.WarnContext(writeCtx, "cache write failed", slog.String("key", key), slog.String("error", err.Error()))
	}
}

func (r *ReadThrough) del(ctx context.Context, key string) {
	delCtx, cancel := context.WithTimeout(ctx, writeTimeout)
	defer cancel()

	if err := r.rdb.Del(delCtx, key).Err(); err != nil {
		r.logger.WarnContext(delCtx, "cache delete failed", slog.String("key", key), slog.String("error", err.Error()))
	}
}

func (o Options) mustBeValid() {
	switch {
	case o.TTL <= 0:
		panic("cache: Options.TTL must be positive")
	case o.AbsentTTL <= 0:
		panic("cache: Options.AbsentTTL must be positive")
	case o.LoadTimeout <= 0:
		panic("cache: Options.LoadTimeout must be positive")
	case o.Deviation < 0 || o.Deviation >= 1:
		panic("cache: Options.Deviation must be in [0, 1)")
	}
}

func jitter(base time.Duration, deviation float64) time.Duration {
	//nolint:gosec // TTL jitter, not a security decision
	return time.Duration((1 + deviation - 2*deviation*rand.Float64()) * float64(base))
}
