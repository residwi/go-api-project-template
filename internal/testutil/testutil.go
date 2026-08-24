package testutil

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

	containerReadyTimeout = 90 * time.Second

	transientGrace = 20 * time.Second
)

func DiscardLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

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

func MustStartPostgres(dbName string) (*pgxpool.Pool, func()) {
	ctx := context.Background()

	dt, err := dockertest.NewPool("")
	if err != nil {
		harnessLogger().Error("testutil: dockertest.NewPool", slog.String("error", err.Error()))
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
		Cmd: []string{"postgres", "-c", "max_connections=300"},
	})

	port := resource.GetPort("5432/tcp")
	adminDSN := fmt.Sprintf("postgres://test:test@localhost:%s/postgres?sslmode=disable", port)
	dsn := fmt.Sprintf("postgres://test:test@localhost:%s/%s?sslmode=disable", port, dbName)

	var adminConn *pgx.Conn
	if retryErr := dt.Retry(func() error {
		conn, e := pgx.Connect(ctx, adminDSN)
		if e != nil {
			return e
		}
		adminConn = conn
		return nil
	}); retryErr != nil {
		harnessLogger().Error("testutil: waiting for postgres", slog.String("error", retryErr.Error()))
		os.Exit(1)
	}

	ensureErr := ensureDatabase(ctx, adminConn, dbName, dsn)
	_ = adminConn.Close(ctx)
	if ensureErr != nil {
		harnessLogger().Error(
			"testutil: ensuring database",
			slog.String("db", dbName),
			slog.String("error", ensureErr.Error()),
		)
		os.Exit(1)
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		harnessLogger().Error(
			"testutil: building package pool",
			slog.String("db", dbName),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	if retryErr := dt.Retry(func() error { return pool.Ping(ctx) }); retryErr != nil {
		harnessLogger().Error(
			"testutil: connecting to package db",
			slog.String("db", dbName),
			slog.String("error", retryErr.Error()),
		)
		os.Exit(1)
	}

	return pool, func() {
		pool.Close()
	}
}

func ensureDatabase(ctx context.Context, admin *pgx.Conn, dbName, dsn string) error {
	if _, lockErr := admin.Exec(ctx, `SELECT pg_advisory_lock(hashtext($1))`, dbName); lockErr != nil {
		return fmt.Errorf("locking for database create: %w", lockErr)
	}
	defer func() { _, _ = admin.Exec(ctx, `SELECT pg_advisory_unlock(hashtext($1))`, dbName) }()

	var exists bool
	if scanErr := admin.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, dbName,
	).Scan(&exists); scanErr != nil {
		return fmt.Errorf("checking for database: %w", scanErr)
	}

	if !exists {
		if _, execErr := admin.Exec(ctx, `CREATE DATABASE "`+dbName+`"`); execErr != nil {
			return fmt.Errorf("creating database %s: %w", dbName, execErr)
		}
	}

	migratePool, poolErr := pgxpool.New(ctx, dsn)
	if poolErr != nil {
		return fmt.Errorf("connecting to migrate %s: %w", dbName, poolErr)
	}
	defer migratePool.Close()
	runMigrations(ctx, migratePool)

	return nil
}

func MustStartRedis(dbIndex int) (*redis.Client, func()) {
	ctx := context.Background()

	dt, err := dockertest.NewPool("")
	if err != nil {
		harnessLogger().Error("testutil: dockertest.NewPool", slog.String("error", err.Error()))
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
		harnessLogger().Error("testutil: waiting for redis", slog.String("error", retryErr.Error()))
		os.Exit(1)
	}

	return client, func() {
		_ = client.Close()
	}
}

func ResetDB(t testing.TB, pool *pgxpool.Pool) {
	t.Helper()
	ctx := context.Background()

	var tableList string
	err := pool.QueryRow(ctx, `
		SELECT string_agg(quote_ident(tablename), ', ')
		FROM pg_tables
		WHERE schemaname = 'public' AND tablename != 'goose_db_version'
	`).Scan(&tableList)
	if err != nil || tableList == "" {
		return
	}

	if _, execErr := pool.Exec(ctx, "TRUNCATE "+tableList+" RESTART IDENTITY CASCADE"); execErr != nil {
		t.Fatalf("testutil: ResetDB: %v", execErr)
	}
}

func ResetRedis(t testing.TB, client *redis.Client) {
	t.Helper()
	if err := client.FlushDB(context.Background()).Err(); err != nil {
		t.Fatalf("testutil: ResetRedis: %v", err)
	}
}

func getOrCreateContainer(dt *dockertest.Pool, name, portID string, opts *dockertest.RunOptions) *dockertest.Resource {
	start := time.Now()
	for {
		if resource, found := dt.ContainerByName(name); found {
			state := resource.Container.State
			if state.Running && resource.GetPort(portID) != "" {
				return resource
			}
			if !isComingUp(state) || time.Since(start) >= transientGrace {
				_ = dt.Purge(resource)
			}
		} else if _, err := dt.RunWithOptions(opts, func(cfg *docker.HostConfig) {
			cfg.AutoRemove = false
			cfg.RestartPolicy = docker.RestartPolicy{Name: "no"}
		}); err != nil && !isAlreadyExists(err) {
			harnessLogger().Error("testutil: starting container", slog.String("name", name), slog.String("error", err.Error()))
			os.Exit(1)
		}

		if time.Since(start) > containerReadyTimeout {
			harnessLogger().Error(
				"testutil: container never became ready",
				slog.String("name", name),
				slog.Duration("waited", time.Since(start)),
			)
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

	goose.SetLogger(goose.NopLogger())

	if err := goose.SetDialect("postgres"); err != nil {
		_ = db.Close()
		harnessLogger().ErrorContext(ctx, "testutil: goose.SetDialect", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		_ = db.Close()
		harnessLogger().ErrorContext(
			ctx,
			"testutil: goose.Up",
			slog.String("dir", migrationsDir),
			slog.String("error", err.Error()),
		)
		os.Exit(1)
	}
	_ = db.Close()
}
