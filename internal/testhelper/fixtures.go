package testhelper

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

type SeedUserOpts struct {
	FirstName string
	LastName  string

	Role string
}

func SeedUser(t testing.TB, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	return SeedUserWith(t, pool, SeedUserOpts{})
}

func SeedUserWith(t testing.TB, pool *pgxpool.Pool, opts SeedUserOpts) uuid.UUID {
	t.Helper()
	ctx := context.Background()

	id := uuid.New()
	firstName := opts.FirstName
	if firstName == "" {
		firstName = "A"
	}
	lastName := opts.LastName
	if lastName == "" {
		lastName = "B"
	}
	role := opts.Role
	if role == "" {
		role = "user"
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO users (id, email, password_hash, first_name, last_name, role)
		 VALUES ($1, $2, 'x', $3, $4, $5)`,
		id, id.String()+"@test.com", firstName, lastName, role,
	); err != nil {
		t.Fatalf("testhelper: SeedUserWith: %v", err)
	}

	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})

	return id
}
