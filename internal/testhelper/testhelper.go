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

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	dockertest "github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/pressly/goose/v3"
	"github.com/redis/go-redis/v9"
)

const (
	postgresContainerName  = "go-api-test-postgres"
	redisContainerName     = "go-api-test-redis"
	containerExpireSeconds = 600
)

// Redis DB index per package (must be unique, 0–15):
//
//	0 — internal/platform/cache
//	1 — internal/middleware
//	2 — internal/features/user
//	3 — internal/server

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
// running so subsequent test binaries can reuse it.
func MustStartPostgres(dbName string) (*pgxpool.Pool, func()) {
	ctx := context.Background()

	dt, err := dockertest.NewPool("")
	if err != nil {
		slog.Error("testhelper: dockertest.NewPool", "error", err)
		os.Exit(1)
	}
	dt.MaxWait = 60e9

	resource := getOrCreatePostgres(dt)
	_ = resource.Expire(containerExpireSeconds)

	port := resource.GetPort("5432/tcp")
	adminDSN := fmt.Sprintf("postgres://test:test@localhost:%s/postgres?sslmode=disable", port)

	var adminPool *pgxpool.Pool
	if retryErr := dt.Retry(func() error {
		var e error
		adminPool, e = pgxpool.New(ctx, adminDSN)
		if e != nil {
			return e
		}
		return adminPool.Ping(ctx)
	}); retryErr != nil {
		slog.Error("testhelper: waiting for postgres", "error", retryErr)
		os.Exit(1)
	}

	// Always drop-then-recreate so we start with a clean schema.
	_, _ = adminPool.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
	if _, execErr := adminPool.Exec(ctx, "CREATE DATABASE "+dbName); execErr != nil {
		slog.Error("testhelper: creating database", "db", dbName, "error", execErr)
		os.Exit(1)
	}
	adminPool.Close()

	dsn := fmt.Sprintf("postgres://test:test@localhost:%s/%s?sslmode=disable", port, dbName)
	var pool *pgxpool.Pool
	if retryErr := dt.Retry(func() error {
		var e error
		pool, e = pgxpool.New(ctx, dsn)
		if e != nil {
			return e
		}
		return pool.Ping(ctx)
	}); retryErr != nil {
		slog.Error("testhelper: connecting to package db", "db", dbName, "error", retryErr)
		os.Exit(1)
	}

	runMigrations(ctx, pool)

	return pool, func() {
		pool.Close()
		admin, adminErr := pgxpool.New(ctx, adminDSN)
		if adminErr == nil {
			_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+dbName+" WITH (FORCE)")
			admin.Close()
		}
	}
}

// MustStartRedis attaches to (or starts) the shared named Redis container and
// returns a client configured to use dbIndex, plus a cleanup func. The container
// is left running after cleanup.
func MustStartRedis(dbIndex int) (*redis.Client, func()) {
	ctx := context.Background()

	dt, err := dockertest.NewPool("")
	if err != nil {
		slog.Error("testhelper: dockertest.NewPool", "error", err)
		os.Exit(1)
	}
	dt.MaxWait = 30e9

	resource := getOrCreateRedis(dt)
	_ = resource.Expire(containerExpireSeconds)

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

func getOrCreatePostgres(dt *dockertest.Pool) *dockertest.Resource {
	if resource, ok := dt.ContainerByName(postgresContainerName); ok {
		if resource.Container.State.Running {
			return resource
		}
		_ = dt.Purge(resource)
	}

	resource, err := dt.RunWithOptions(&dockertest.RunOptions{
		Name:       postgresContainerName,
		Repository: "postgres",
		Tag:        "18-alpine",
		Env: []string{
			"POSTGRES_USER=test",
			"POSTGRES_PASSWORD=test",
			"POSTGRES_DB=postgres",
			"listen_addresses='*'",
		},
	}, func(cfg *docker.HostConfig) {
		cfg.AutoRemove = false
		cfg.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		if strings.Contains(err.Error(), "Conflict") || strings.Contains(err.Error(), "already exists") {
			for attempt := 1; attempt <= 5; attempt++ {
				time.Sleep(time.Duration(200*attempt) * time.Millisecond)
				if r, ok := dt.ContainerByName(postgresContainerName); ok {
					return r
				}
			}
		}
		slog.Error("testhelper: starting postgres container", "error", err)
		os.Exit(1)
	}
	return resource
}

func getOrCreateRedis(dt *dockertest.Pool) *dockertest.Resource {
	if resource, ok := dt.ContainerByName(redisContainerName); ok {
		if resource.Container.State.Running {
			return resource
		}
		_ = dt.Purge(resource)
	}

	resource, err := dt.RunWithOptions(&dockertest.RunOptions{
		Name:       redisContainerName,
		Repository: "redis",
		Tag:        "8-alpine",
	})
	if err != nil {
		if strings.Contains(err.Error(), "Conflict") || strings.Contains(err.Error(), "already exists") {
			for attempt := 1; attempt <= 5; attempt++ {
				time.Sleep(time.Duration(200*attempt) * time.Millisecond)
				if r, ok := dt.ContainerByName(redisContainerName); ok {
					return r
				}
			}
		}
		slog.Error("testhelper: starting redis container", "error", err)
		os.Exit(1)
	}
	return resource
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
