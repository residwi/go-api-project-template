package testhelper

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Two rules keep these fixtures from becoming a liability:
//
//   - Raw SQL only. Seeding through a repository or service would run the code
//     under test, so a bug there could produce a passing test over wrong data.
//   - No feature package imported, and no feature type in a signature. Most test
//     packages import testhelper, so a dependency on features would make a cycle
//     inevitable. Callers wanting a domain object read one back themselves.

// SeedUserOpts is all-optional: the zero value produces exactly the row SeedUser
// does, so a caller names only what its test cares about.
type SeedUserOpts struct {
	// FirstName and LastName default to "A" and "B". No test asserts on them;
	// they exist because the users table requires them NOT NULL.
	FirstName string
	LastName  string

	// Role defaults to "user", which is also the column default. Set it to
	// "admin" for a user that role-filtered queries should match.
	Role string
}

// SeedUser inserts an active user with role "user" and returns its id. The email
// is derived from that id because users.email is UNIQUE and unrelated tests share
// a database. The row is cleaned up for the packages that do not call [ResetDB].
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
