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

	// Every package binary races for the same container name; the loser waits
	// here for the winner to publish its port.
	containerReadyTimeout = 90 * time.Second

	// Past this, a still-not-running container is treated as abandoned rather
	// than starting up, and replaced.
	transientGrace = 20 * time.Second
)

// Redis DB index per package (must be unique, 0–15):
//
//	0 — internal/platform/cache
//	1 — internal/transport/http/middleware
//	2 — internal/modules/user/postgres
//	3 — internal/transport/http
//	5 — test/e2e
//	6 — internal/modules/user/redis

// DiscardLogger makes a caller say out loud that it wants log output thrown
// away. Nothing in the suite asserts on it.
func DiscardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

// harnessLogger reports the bootstrap failures below. A function, not a var,
// because sloglint's no-global rule forbids the var and this runs from TestMain
// before anything can inject a logger.
func harnessLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func init() {
	if os.Getenv("DOCKER_HOST") != "" {
		return
	}
	candidates := []string{
		filepath.Join(os.Getenv("HOME"), ".orbstack", "run", "docker.sock"),
		filepath.Join(os.Getenv("HOME"), ".docker", "run", "docker.sock"),
	}
	for _, candidate := range candidates {
		if _, statErr := os.Stat( //nolint:gosec // G703: path built from known constant suffixes and HOME env var
			candidate,
		); statErr == nil {
			_ = os.Setenv("DOCKER_HOST", "unix://"+candidate)
			return
		}
	}
}

// MustStartPostgres creates a fresh migrated database named dbName and returns a
// pool plus a cleanup that drops it. The shared container is left running for
// the next test binary; remove it with `make test-clean`.
func MustStartPostgres(dbName string) (*pgxpool.Pool, func()) {
	ctx := context.Background()

	dt, err := dockertest.NewPool("")
	if err != nil {
		harnessLogger().Error("testhelper: dockertest.NewPool", slog.Any("error", err))
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
		// The default 100 is not enough: `go test` runs GOMAXPROCS package
		// binaries at once and each opens its own pool.
		Cmd: []string{"postgres", "-c", "max_connections=300"},
	})

	port := resource.GetPort("5432/tcp")
	adminDSN := fmt.Sprintf("postgres://test:test@localhost:%s/postgres?sslmode=disable", port)

	// The retry wraps the *connect*, never the DROP/CREATE pair below.
	//
	// Every package binary dials this container at once, so single dials come
	// back "connection reset by peer". Holding one pgx.Conn across both
	// statements means no dial can happen between them; a pool would not, since
	// it acquires lazily and Ping only proves one connection worked once.
	//
	// Never repeat the DROP: WITH (FORCE) calls pg_terminate_backend, so a retry
	// turns one transient error into a termination storm on a container every
	// other package is using.
	var adminConn *pgx.Conn
	if retryErr := dt.Retry(func() error {
		conn, e := pgx.Connect(ctx, adminDSN)
		if e != nil {
			return e
		}
		adminConn = conn
		return nil
	}); retryErr != nil {
		harnessLogger().Error("testhelper: waiting for postgres", slog.Any("error", retryErr))
		os.Exit(1)
	}

	// Not fatal -- IF EXISTS makes it a no-op on a clean cluster -- but not
	// swallowed either: it is the usual explanation for the CREATE below failing
	// with "already exists".
	if _, dropErr := adminConn.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)"); dropErr != nil {
		harnessLogger().Warn("testhelper: dropping stale database", slog.Any("db", dbName), slog.Any("error", dropErr))
	}
	if _, execErr := adminConn.Exec(ctx, "CREATE DATABASE "+dbName); execErr != nil {
		harnessLogger().Error("testhelper: creating database", slog.Any("db", dbName), slog.Any("error", execErr))
		os.Exit(1)
	}
	_ = adminConn.Close(ctx)

	dsn := fmt.Sprintf("postgres://test:test@localhost:%s/%s?sslmode=disable", port, dbName)
	// Outside the retry: pgxpool.New only parses the DSN, so retrying it leaked a
	// pool and its goroutines on every attempt.
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		harnessLogger().Error("testhelper: building package pool", slog.Any("db", dbName), slog.Any("error", err))
		os.Exit(1)
	}
	if retryErr := dt.Retry(func() error { return pool.Ping(ctx) }); retryErr != nil {
		harnessLogger().Error("testhelper: connecting to package db", slog.Any("db", dbName), slog.Any("error", retryErr))
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
		harnessLogger().Error("testhelper: dockertest.NewPool", slog.Any("error", err))
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
		harnessLogger().Error("testhelper: waiting for redis", slog.Any("error", retryErr))
		os.Exit(1)
	}

	return client, func() {
		_ = client.Close()
	}
}

// ResetDB truncates every table in the public schema and restarts sequences.
// Call it at the start of each subtest.
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

// ResetRedis flushes the client's selected database. Call it at the start of
// each subtest.
func ResetRedis(t testing.TB, client *redis.Client) {
	t.Helper()
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("testhelper: ResetRedis: %v", err)
	}
}

// getOrCreateContainer returns the shared named container only once it is
// running with portID published.
//
// Never call resource.Expire() here. It is not a keep-alive TTL: it issues an
// immediate `stop`, and the argument is only Docker's grace period before
// SIGKILL. Neither container exits on stop while test binaries hold
// connections, so Expire(600) was a guaranteed SIGKILL ten minutes in --
// killing the shared container mid-run for every other package. A pending stop
// cannot be cancelled from a later package either. The containers are meant to
// outlive the run; `make test-clean` removes them.
//
// Every package binary runs this concurrently against the same name, so losing
// any race is survivable: a container in a transient state is waited for rather
// than purged (purging anything merely "not running" deletes one another binary
// just started), a "Conflict" from RunWithOptions means someone else won the
// create, and the published port is checked before returning -- returning on
// name alone yielded DSNs like "postgres://test:test@localhost:/postgres".
func getOrCreateContainer(dt *dockertest.Pool, name, portID string, opts *dockertest.RunOptions) *dockertest.Resource {
	start := time.Now()
	for {
		if resource, found := dt.ContainerByName(name); found {
			state := resource.Container.State
			if state.Running && resource.GetPort(portID) != "" {
				return resource
			}
			// Purge only what is genuinely dead: one still coming up was started
			// by another binary moments ago. transientGrace bounds the patience,
			// for the binary that died between create and start.
			if !isComingUp(state) || time.Since(start) >= transientGrace {
				_ = dt.Purge(resource)
			}
		} else if _, err := dt.RunWithOptions(opts, func(cfg *docker.HostConfig) {
			cfg.AutoRemove = false
			cfg.RestartPolicy = docker.RestartPolicy{Name: "no"}
		}); err != nil && !isAlreadyExists(err) {
			harnessLogger().Error("testhelper: starting container", slog.Any("name", name), slog.Any("error", err))
			os.Exit(1)
		}

		if time.Since(start) > containerReadyTimeout {
			harnessLogger().Error("testhelper: container never became ready", slog.Any("name", name), slog.Any("waited", time.Since(start)))
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

	// goose logs an "OK <file>" line per migration through the standard log
	// package, and `make test` runs -v across ~20 packages that migrate, which
	// buries the failures worth reading. Silencing it hides nothing: UpContext
	// still returns its error and the call below reports it.
	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		_ = db.Close()
		harnessLogger().ErrorContext(ctx, "testhelper: goose.SetDialect", slog.Any("error", err))
		os.Exit(1)
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		_ = db.Close()
		harnessLogger().ErrorContext(ctx, "testhelper: goose.Up", slog.Any("dir", migrationsDir), slog.Any("error", err))
		os.Exit(1)
	}
	_ = db.Close()
}
