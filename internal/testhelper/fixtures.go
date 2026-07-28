package testhelper

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Fixtures in this file exist so that a row every feature's tests need -- a
// user to hang a foreign key off -- is written down once instead of once per
// package. Two rules keep them from becoming a liability:
//
//   - They are raw SQL. A fixture that seeded through a repository or service
//     would run the code under test, so a bug there would produce a passing
//     test over wrong data. The INSERT must be independent of the thing it is
//     checking.
//
//   - They import no feature package, and no feature type appears in their
//     signatures. internal/testhelper is imported by most test packages in the
//     tree; taking a dependency on features would turn it into a hub and make
//     an import cycle inevitable the first time a feature wanted a fixture.
//     Callers that want a domain object read one back themselves, in their own
//     package, where importing the feature is free.

// SeedUserOpts overrides what SeedUserWith would otherwise pick. Every field is
// optional -- the zero value produces exactly the row SeedUser produces -- so a
// caller only names the attribute its test actually cares about.
type SeedUserOpts struct {
	// FirstName and LastName default to "A" and "B". No test asserts on them;
	// they exist because the users table requires them NOT NULL.
	FirstName string
	LastName  string

	// Role defaults to "user", which is also the column default. Set it to
	// "admin" for a user that role-filtered queries should match.
	Role string
}

// SeedUser inserts a minimal valid user and returns its id. The user is active
// with role "user", and its email is derived from the generated id, because
// users.email is UNIQUE and unrelated tests share a database.
//
// The row is deleted when the test that seeded it finishes. Most callers also
// call [ResetDB], which truncates it anyway; the cleanup is here for the
// packages that do not.
func SeedUser(t testing.TB, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	return SeedUserWith(t, pool, SeedUserOpts{})
}

// SeedUserWith is [SeedUser] with the defaults it applies made explicit. See
// [SeedUserOpts] for which fields are worth setting.
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
