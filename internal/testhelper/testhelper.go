package testhelper

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	dockertest "github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
)

const (
	postgresContainerName = "go-api-test-postgres"
	redisContainerName    = "go-api-test-redis"

	// How long to wait for a shared container to become usable. Every package
	// binary races for the same container name, so the loser of the create race
	// waits here for the winner's container to publish its port.
	containerReadyTimeout = 90 * time.Second

	// How long a container may sit in a not-yet-running state before we stop
	// assuming another binary is bringing it up and replace it instead.
	transientGrace = 20 * time.Second
)

// Redis DB index per package (must be unique, 0–15):
//
//	0 — internal/platform/cache
//	1 — internal/transport/http/middleware
//	2 — internal/user/postgres
//	3 — internal/transport/http
//	4 — internal/user

func init() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))

	if os.Getenv("DOCKER_HOST") != "" {
		return
	}
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".orbstack", "run", "docker.sock"),
		filepath.Join(os.Getenv("HOME"), ".docker", "run", "docker.sock"),
	}
	for _, candidate := range candidates {
		if _, statErr := os.Stat(candidate); statErr == nil { //nolint:gosec // G703: path built from known constant suffixes and HOME env var
			_ = os.Setenv("DOCKER_HOST", "unix://"+candidate)
			return
		}
	}
}

// MustStartPostgres attaches to (or starts) the shared named Postgres container,
// creates a fresh database named dbName, runs all up-migrations, and returns a
// pool plus a cleanup func that drops the database. The container itself is left
// running so subsequent test binaries can reuse it; remove it with
// `make test-clean`.
func MustStartPostgres(dbName string) (*pgxpool.Pool, func()) {
	ctx := context.Background()

	dt, err := dockertest.NewPool("")
	if err != nil {
		slog.Error("testhelper: dockertest.NewPool", "error", err)
		os.Exit(1)
	}
	dt.MaxWait = 60e9

	resource := getOrCreateContainer(dt, postgresContainerName, "5432/tcp", &dockertest.RunOptions{
		Name:       postgresContainerName,
		Repository: "postgres",
		Tag:        "18-alpine",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=postgres",
		},
		// Default max_connections=100 is not enough once enough packages create
		// their own database/pool concurrently (go test's default -p is
		// GOMAXPROCS-wide); raise the ceiling so `make test` has headroom as
		// more packages adopt this pattern.
		Cmd: []string{"postgres", "-c", "max_connections=300"},
	})

	port := resource.GetPort("5432/tcp")
	adminDSN := fmt.Sprintf("postgres://test:test@localhost:%s/postgres?sslmode=disable", port)

	// Always drop-then-recreate so we start with a clean schema.
	//
	// The retry is around *establishing a connection*, and deliberately not
	// around the DROP/CREATE pair. Both halves of that split are load-bearing:
	//
	//   - Retrying the connect is the actual fix. `go test` starts one binary
	//     per package, GOMAXPROCS of them at a time, and they all dial this
	//     container at once, so individual dials come back "connection reset by
	//     peer" / "unexpected EOF". The previous code proved liveness with
	//     pgxpool.Ping, which is not enough: a pool acquires lazily, so Ping
	//     shows that *a* connection worked at that instant and the following
	//     Exec was still free to open a new one and lose the race. pgx.Connect
	//     hands back an established connection, and holding that one connection
	//     across both statements means no dial can happen between them.
	//
	//   - Retrying the DROP/CREATE pair was measured to make a full `make test`
	//     worse and was reverted in 51ec7c8. DROP DATABASE ... WITH (FORCE)
	//     calls pg_terminate_backend, so re-running the pair turns one
	//     transient error into a backend-termination storm against a container
	//     every other package is using. Never repeat the DROP.
	var adminConn *pgx.Conn
	if retryErr := dt.Retry(func() error {
		conn, e := pgx.Connect(ctx, adminDSN)
		if e != nil {
			return e
		}
		adminConn = conn
		return nil
	}); retryErr != nil {
		slog.Error("testhelper: waiting for postgres", "error", retryErr)
		os.Exit(1)
	}

	// A failing DROP is not fatal by itself -- IF EXISTS makes it a no-op on a
	// clean cluster -- but it must not be swallowed either, because it is the
	// usual explanation for the CREATE below failing with "already exists".
	if _, dropErr := adminConn.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)"); dropErr != nil {
		slog.Warn("testhelper: dropping stale database", "db", dbName, "error", dropErr)
	}
	if _, execErr := adminConn.Exec(ctx, "CREATE DATABASE "+dbName); execErr != nil {
		slog.Error("testhelper: creating database", "db", dbName, "error", execErr)
		os.Exit(1)
	}
	_ = adminConn.Close(ctx)

	dsn := fmt.Sprintf("postgres://test:test@localhost:%s/%s?sslmode=disable", port, dbName)
	// Built once, outside the retry: pgxpool.New only parses the DSN and cannot
	// fail transiently, so retrying it leaked a pool (and its background
	// goroutines) on every attempt.
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		slog.Error("testhelper: building package pool", "db", dbName, "error", err)
		os.Exit(1)
	}
	if retryErr := dt.Retry(func() error { return pool.Ping(ctx) }); retryErr != nil {
		slog.Error("testhelper: connecting to package db", "db", dbName, "error", retryErr)
		os.Exit(1)
	}

	runMigrations(ctx, pool)

	return pool, func() {
		pool.Close()
		conn, connErr := pgx.Connect(ctx, adminDSN)
		if connErr == nil {
			_, _ = conn.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
			_ = conn.Close(ctx)
		}
	}
}

// MustStartRedis attaches to (or starts) the shared named Redis container and
// returns a client configured to use dbIndex, plus a cleanup func. The container
// is left running after cleanup; remove it with `make test-clean`.
func MustStartRedis(dbIndex int) (*redis.Client, func()) {
	ctx := context.Background()

	dt, err := dockertest.NewPool("")
	if err != nil {
		slog.Error("testhelper: dockertest.NewPool", "error", err)
		os.Exit(1)
	}
	dt.MaxWait = 30e9

	resource := getOrCreateContainer(dt, redisContainerName, "6379/tcp", &dockertest.RunOptions{
		Name:       redisContainerName,
		Repository: "redis",
		Tag:        "8-alpine",
	})

	addr := fmt.Sprintf("localhost:%s", resource.GetPort("6379/tcp"))
	var client *redis.Client
	if retryErr := dt.Retry(func() error {
		client = redis.NewClient(&redis.Options{Addr: addr, DB: dbIndex})
		return client.Ping(ctx).Err()
	}); retryErr != nil {
		slog.Error("testhelper: waiting for redis", "error", retryErr)
		os.Exit(1)
	}

	return client, func() {
		_ = client.Close()
	}
}

// ResetDB truncates all user tables in the public schema and restarts sequences.
// Call it at the start of each subtest to get a clean state.
func ResetDB(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	var tableList string
	err := pool.QueryRow(ctx, `
		SELECT string_agg(quote_ident(tablename), ', ')
		FROM pg_tables
		WHERE schemaname = 'public'
	`).Scan(&tableList)
	if err != nil || tableList == "" {
		return
	}

	if _, execErr := pool.Exec(ctx, "TRUNCATE "+tableList+" RESTART IDENTITY CASCADE"); execErr != nil {
		t.Fatalf("testhelper: ResetDB: %v", execErr)
	}
}

// ResetRedis flushes all keys in the client's selected database.
// Call it at the start of each subtest to get a clean state.
func ResetRedis(t testing.TB, client *redis.Client) {
	t.Helper()
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("testhelper: ResetRedis: %v", err)
	}
}

// getOrCreateContainer attaches to the shared named container, creating it if it
// is absent, and returns it only once it is running with portID published.
//
// It deliberately does NOT call resource.Expire(). Expire is not a
// keep-alive-style TTL: it issues `POST /containers/{id}/stop?t=seconds` in a
// goroutine, which is an *immediate* stop request whose argument is only the
// grace period Docker waits before escalating to SIGKILL. Neither container
// exits on its stop signal while test binaries hold connections, so the effect
// of `Expire(600)` was a guaranteed SIGKILL 600 seconds after the container was
// created. Docker's own event log shows it exactly:
//
//	kill signal:9
//	stop  (x8 -- one per still-blocked Expire goroutine)
//	die   execDuration:600 exitCode:137
//
// That killed the shared container mid-run roughly every ten minutes, which was
// every failing package in a full `make test`: `connection refused` against the
// container port, and `FATAL: terminating connection due to unexpected
// postmaster exit (SQLSTATE 57P01)`. Calling Expire again from a later package
// cannot undo it either -- a pending docker stop cannot be cancelled, and the
// earliest deadline wins. The containers are meant to outlive the run so the
// next one can reuse them; `make test-clean` removes them.
//
// Every package binary runs this concurrently against the same container name,
// so it is written to tolerate losing every race:
//
//   - A container in a transient state is waited for, never purged. Purging
//     anything merely "not running" was a real hazard: a binary that inspected
//     the container during the few milliseconds it sat in "created" would delete
//     a container another binary had just started and was about to use.
//   - "Conflict"/"already exists" from RunWithOptions just means another binary
//     won the create, so we loop and attach to its container instead.
//   - The published port is checked before returning. The previous conflict
//     fallback returned as soon as a container existed by name, so GetPort could
//     still be "" and yield a DSN like "postgres://test:test@localhost:/postgres".
func getOrCreateContainer(dt *dockertest.Pool, name, portID string, opts *dockertest.RunOptions) *dockertest.Resource {
	start := time.Now()
	for {
		if resource, found := dt.ContainerByName(name); found {
			state := resource.Container.State
			if state.Running && resource.GetPort(portID) != "" {
				return resource
			}
			// Purge only a container that is genuinely dead. If it is still
			// coming up, another binary started it moments ago and is about to
			// use it. transientGrace bounds that patience, in case a binary died
			// between create and start and left the container wedged.
			if !isComingUp(state) || time.Since(start) >= transientGrace {
				_ = dt.Purge(resource)
			}
		} else if _, err := dt.RunWithOptions(opts, func(cfg *docker.HostConfig) {
			cfg.AutoRemove = false
			cfg.RestartPolicy = docker.RestartPolicy{Name: "no"}
		}); err != nil && !isAlreadyExists(err) {
			slog.Error("testhelper: starting container", "name", name, "error", err)
			os.Exit(1)
		}

		if time.Since(start) > containerReadyTimeout {
			slog.Error("testhelper: container never became ready", "name", name, "waited", time.Since(start))
			os.Exit(1)
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func isComingUp(state docker.State) bool {
	return state.Running || state.Restarting || state.Paused || state.Status == "created"
}

func isAlreadyExists(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "Conflict") || strings.Contains(msg, "already exists")
}

func runMigrations(ctx context.Context, pool *pgxpool.Pool) {
	_, file, _, _ := runtime.Caller(0)
	migrationsDir := filepath.Join(filepath.Dir(file), "..", "..", "db", "migrations")

	db := stdlib.OpenDBFromPool(pool)

	if err := goose.SetDialect("postgres"); err != nil {
		_ = db.Close()
		slog.ErrorContext(ctx, "testhelper: goose.SetDialect", "error", err)
		os.Exit(1)
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		_ = db.Close()
		slog.ErrorContext(ctx, "testhelper: goose.Up", "dir", migrationsDir, "error", err)
		os.Exit(1)
	}
	_ = db.Close()
}
