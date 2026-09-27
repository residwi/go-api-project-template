package postgres

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/testutil"
)

// This package shares test_user with every other user postgres access in
// the module; see the registry comment in internal/testutil. It never
// resets or truncates -- every row it touches is seeded here with a fresh
// uuid.New() and cleaned up by name.

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testutil.MustStartPostgres("test_user")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates user", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		u := &domain.User{
			Email:        uuid.New().String() + "@example.com",
			PasswordHash: "hashed",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "user",
			Active:       true,
		}

		err := repo.Create(context.Background(), u)
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, u.ID)
		assert.False(t, u.CreatedAt.IsZero())
		t.Cleanup(func() {
			testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, u.ID)
		})
	})

	t.Run("returns conflict on duplicate email", func(t *testing.T) {
		existing := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		dup := &domain.User{
			Email:        existing.Email,
			PasswordHash: "hashed",
			FirstName:    "Jane",
			LastName:     "Doe",
			Role:         "user",
			Active:       true,
		}
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns user", func(t *testing.T) {
		u := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.ID, got.ID)
		assert.Equal(t, u.Email, got.Email)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestPostgresRepository_GetByEmail(t *testing.T) {
	t.Run("returns user by email", func(t *testing.T) {
		u := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		got, err := repo.GetByEmail(context.Background(), u.Email)
		require.NoError(t, err)
		assert.Equal(t, u.ID, got.ID)
		assert.Equal(t, u.Email, got.Email)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, err := repo.GetByEmail(context.Background(), "nobody-"+uuid.New().String()+"@nowhere.example")
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns the users this subtest seeded", func(t *testing.T) {
		token := "listadmin-" + uuid.New().String()[:8]
		u1 := seedUserWithEmailToken(t, token)
		u2 := seedUserWithEmailToken(t, token)
		repo := New(database.DB{Primary: testPool})

		users, total, err := repo.ListAdmin(context.Background(), user.AdminListParams{
			Page: 1, PageSize: 50,
			Search: token,
		})
		require.NoError(t, err)
		assert.Equal(t, 2, total)

		ids := make([]uuid.UUID, len(users))
		for i, u := range users {
			ids[i] = u.ID
		}
		assert.Contains(t, ids, u1.ID)
		assert.Contains(t, ids, u2.ID)
	})

	// Page 2 is what proves the OFFSET is wired: with page size 1 and three
	// users scoped to this subtest's own search token, a repo that passed the
	// raw page number (or dropped the -1) would return the wrong row while
	// still returning one row and the right total.
	t.Run("page 2 skips the first page's rows", func(t *testing.T) {
		token := "page2-" + uuid.New().String()[:8]
		seedUserWithEmailToken(t, token)
		seedUserWithEmailToken(t, token)
		seedUserWithEmailToken(t, token)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		first, total, err := repo.ListAdmin(ctx, user.AdminListParams{
			Page: 1, PageSize: 1, Search: token,
		})
		require.NoError(t, err)
		require.Len(t, first, 1)
		assert.Equal(t, 3, total)

		second, _, err := repo.ListAdmin(ctx, user.AdminListParams{
			Page: 2, PageSize: 1, Search: token,
		})
		require.NoError(t, err)
		require.Len(t, second, 1)
		assert.NotEqual(t, first[0].ID, second[0].ID)
	})

	t.Run("filters by role", func(t *testing.T) {
		u := seedUser(t)
		_, err := testPool.Exec(context.Background(), `UPDATE users SET role = 'admin' WHERE id = $1`, u.ID)
		require.NoError(t, err)
		repo := New(database.DB{Primary: testPool})

		users, total, err := repo.ListAdmin(context.Background(), user.AdminListParams{
			Page: 1, PageSize: 50, Role: "admin",
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, u := range users {
			assert.Equal(t, "admin", u.Role)
		}
	})

	t.Run("filters by active", func(t *testing.T) {
		seedUser(t)
		repo := New(database.DB{Primary: testPool})
		active := true

		users, _, err := repo.ListAdmin(context.Background(), user.AdminListParams{
			Page: 1, PageSize: 50, Active: &active,
		})
		require.NoError(t, err)
		for _, u := range users {
			assert.True(t, u.Active)
		}
	})

	t.Run("filters by search", func(t *testing.T) {
		u := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		users, total, err := repo.ListAdmin(context.Background(), user.AdminListParams{
			Page: 1, PageSize: 50, Search: u.Email,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, users)
	})
}

func TestPostgresRepository_UpdateProfile(t *testing.T) {
	t.Run("updates profile fields", func(t *testing.T) {
		u := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		u.FirstName = "Updated"
		u.LastName = "Name"
		err := repo.UpdateProfile(context.Background(), u)
		require.NoError(t, err)

		got, err := repo.GetByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated", got.FirstName)
		assert.Equal(t, "Name", got.LastName)
	})

	t.Run("never writes role or active", func(t *testing.T) {
		u := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		before, err := repo.GetByID(context.Background(), u.ID)
		require.NoError(t, err)

		u.Role = "admin"
		u.Active = !before.Active
		require.NoError(t, repo.UpdateProfile(context.Background(), u))

		got, err := repo.GetByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, before.Role, got.Role)
		assert.Equal(t, before.Active, got.Active)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		u := &domain.User{
			ID:        uuid.New(),
			FirstName: "Ghost",
			LastName:  "User",
			Role:      "user",
			Active:    true,
		}
		err := repo.UpdateProfile(context.Background(), u)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("soft deletes user", func(t *testing.T) {
		id := testutil.SeedUser(t, testPool)
		repo := New(database.DB{Primary: testPool})

		applied, err := repo.Delete(context.Background(), id)
		require.NoError(t, err)
		require.True(t, applied)
	})

	t.Run("GetByID returns not found after delete", func(t *testing.T) {
		id := testutil.SeedUser(t, testPool)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		applied, err := repo.Delete(ctx, id)
		require.NoError(t, err)
		require.True(t, applied)

		_, err = repo.GetByID(ctx, id)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("reports not applied for nonexistent user", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		applied, err := repo.Delete(context.Background(), uuid.New())
		require.NoError(t, err)
		assert.False(t, applied)
	})
}

func TestPostgresRepository_LastAdminGuard(t *testing.T) {
	t.Run("delete blocked for the last active admin", func(t *testing.T) {
		withAdminFixture(t, func(ctx context.Context, repo *Repository) {
			admin := seedAdmin(ctx, t, true)

			applied, err := repo.Delete(ctx, admin)
			require.NoError(t, err)
			assert.False(t, applied)
		})
	})

	t.Run("delete allowed while another active admin remains", func(t *testing.T) {
		withAdminFixture(t, func(ctx context.Context, repo *Repository) {
			admin := seedAdmin(ctx, t, true)
			seedAdmin(ctx, t, true)

			applied, err := repo.Delete(ctx, admin)
			require.NoError(t, err)
			assert.True(t, applied)
		})
	})

	t.Run("delete allowed for an inactive admin even when no active admin remains", func(t *testing.T) {
		withAdminFixture(t, func(ctx context.Context, repo *Repository) {
			inactive := seedAdmin(ctx, t, false)

			applied, err := repo.Delete(ctx, inactive)
			require.NoError(t, err)
			assert.True(t, applied)
		})
	})

	t.Run("delete still blocked for the last active admin beside an inactive one", func(t *testing.T) {
		withAdminFixture(t, func(ctx context.Context, repo *Repository) {
			active := seedAdmin(ctx, t, true)
			seedAdmin(ctx, t, false)

			applied, err := repo.Delete(ctx, active)
			require.NoError(t, err)
			assert.False(t, applied)
		})
	})

	t.Run("deactivating the last active admin is blocked", func(t *testing.T) {
		withAdminFixture(t, func(ctx context.Context, repo *Repository) {
			admin := seedAdmin(ctx, t, true)

			applied, err := repo.UpdateGuarded(ctx, &domain.User{
				ID: admin, FirstName: "A", LastName: "B", Role: "admin", Active: false,
			})
			require.NoError(t, err)
			assert.False(t, applied)
		})
	})

	t.Run("demoting the last active admin is blocked", func(t *testing.T) {
		withAdminFixture(t, func(ctx context.Context, repo *Repository) {
			admin := seedAdmin(ctx, t, true)

			applied, err := repo.UpdateGuarded(ctx, &domain.User{
				ID: admin, FirstName: "A", LastName: "B", Role: "user", Active: true,
			})
			require.NoError(t, err)
			assert.False(t, applied)
		})
	})

	t.Run("editing an admin that stays an active admin is allowed", func(t *testing.T) {
		withAdminFixture(t, func(ctx context.Context, repo *Repository) {
			admin := seedAdmin(ctx, t, true)

			applied, err := repo.UpdateGuarded(ctx, &domain.User{
				ID: admin, FirstName: "Renamed", LastName: "B", Role: "admin", Active: true,
			})
			require.NoError(t, err)
			assert.True(t, applied)
		})
	})

	t.Run("renaming an inactive admin is allowed while one active admin remains", func(t *testing.T) {
		withAdminFixture(t, func(ctx context.Context, repo *Repository) {
			seedAdmin(ctx, t, true)
			inactive := seedAdmin(ctx, t, false)

			applied, err := repo.UpdateGuarded(ctx, &domain.User{
				ID: inactive, FirstName: "Renamed", LastName: "B", Role: "admin", Active: false,
			})
			require.NoError(t, err)
			assert.True(t, applied)
		})
	})
}

func TestPostgresRepository_LastAdminGuardConcurrency(t *testing.T) {
	t.Run("concurrent demotions of the last two active admins let exactly one through", func(t *testing.T) {
		ctx := context.Background()
		isolateActiveAdmins(t)
		a := seedAdmin(ctx, t, true)
		b := seedAdmin(ctx, t, true)
		t.Cleanup(func() {
			_, err := testPool.Exec(context.Background(), `DELETE FROM users WHERE id = ANY($1)`, []uuid.UUID{a, b})
			require.NoError(t, err)
		})

		blocker, err := testPool.Begin(ctx)
		require.NoError(t, err)
		_, err = blocker.Exec(ctx, `SELECT id FROM users WHERE id = ANY($1) FOR UPDATE`, []uuid.UUID{a, b})
		require.NoError(t, err)

		repo := New(database.DB{Primary: testPool})
		applied := make([]bool, 2)
		errsOut := make([]error, 2)
		var wg sync.WaitGroup
		for i, id := range []uuid.UUID{a, b} {
			wg.Go(func() {
				applied[i], errsOut[i] = repo.UpdateGuarded(ctx, &domain.User{
					ID: id, FirstName: "A", LastName: "B", Role: "user", Active: true,
				})
			})
		}

		waitForLockWaiters(t, 2)
		require.NoError(t, blocker.Commit(ctx))
		wg.Wait()

		require.NoError(t, errsOut[0])
		require.NoError(t, errsOut[1])
		assert.NotEqual(t, applied[0], applied[1], "exactly one demotion must apply")

		var remaining int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT count(*) FROM users WHERE id = ANY($1) AND role = 'admin' AND active`,
			[]uuid.UUID{a, b},
		).Scan(&remaining))
		assert.Equal(t, 1, remaining)
	})
}

// CountAdmins is a global aggregate over a database this package shares with
// every other postgres access in the module, so a concurrent sibling
// subtest's own admin-seeding can bump the count between this subtest's own
// before/after reads. Asserting a lower bound rather than exact equality
// keeps the assertion true regardless of what a concurrent sibling does.
func TestPostgresRepository_CountAdmins(t *testing.T) {
	t.Run("returns a non-negative count", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		count, err := repo.CountAdmins(context.Background())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 0)
	})

	t.Run("increases after seeding an active admin", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		before, err := repo.CountAdmins(ctx)
		require.NoError(t, err)

		testutil.SeedUserWith(t, testPool, testutil.SeedUserOpts{
			FirstName: "Admin",
			LastName:  "User",
			Role:      "admin",
		})

		after, err := repo.CountAdmins(ctx)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, after, before+1)
	})
}

func TestPostgresRepository_IncrementTokenVersion(t *testing.T) {
	t.Run("increments token version", func(t *testing.T) {
		u := seedUser(t)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		err := repo.IncrementTokenVersion(ctx, u.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.TokenVersion+1, got.TokenVersion)
	})

	t.Run("returns not found for missing user", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		err := repo.IncrementTokenVersion(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(database.DB{Primary: testPool})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("Create returns error on cancelled context", func(t *testing.T) {
		u := &domain.User{
			Email:        uuid.New().String() + "@example.com",
			PasswordHash: "hashed",
			FirstName:    "Test",
			LastName:     "User",
			Role:         "user",
			Active:       true,
		}
		err := repo.Create(ctx, u)
		require.Error(t, err)
		assert.NotErrorIs(t, err, errs.ErrConflict)
	})

	t.Run("GetByID returns error on cancelled context", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("GetByEmail returns error on cancelled context", func(t *testing.T) {
		_, err := repo.GetByEmail(ctx, "test@example.com")
		require.Error(t, err)
		assert.NotErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("ListAdmin returns error on cancelled context", func(t *testing.T) {
		_, _, err := repo.ListAdmin(ctx, user.AdminListParams{Page: 1, PageSize: 10})
		require.Error(t, err)
	})

	t.Run("Update returns error on cancelled context", func(t *testing.T) {
		u := &domain.User{
			ID:        uuid.New(),
			FirstName: "Test",
			LastName:  "User",
			Role:      "user",
			Active:    true,
		}
		err := repo.UpdateProfile(ctx, u)
		require.Error(t, err)
		assert.NotErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("Delete returns error on cancelled context", func(t *testing.T) {
		_, err := repo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("CountAdmins returns error on cancelled context", func(t *testing.T) {
		_, err := repo.CountAdmins(ctx)
		require.Error(t, err)
	})

	t.Run("IncrementTokenVersion returns error on cancelled context", func(t *testing.T) {
		err := repo.IncrementTokenVersion(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, errs.ErrNotFound)
	})
}

func seedUser(t *testing.T) *domain.User {
	t.Helper()
	id := testutil.SeedUser(t, testPool)

	repo := New(database.DB{Primary: testPool})
	u, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return u
}

// seedUserWithEmailToken seeds a user whose email contains token, so a
// Search-scoped ListAdmin query returns exactly the rows a subtest seeded
// itself, regardless of what else test_user holds.
func seedUserWithEmailToken(t *testing.T, token string) *domain.User {
	t.Helper()
	id := uuid.New()
	email := token + "-" + id.String() + "@test.com"
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, first_name, last_name, role)
		VALUES ($1, $2, 'x', 'A', 'B', 'user')`,
		id, email,
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})

	repo := New(database.DB{Primary: testPool})
	u, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return u
}

// withAdminFixture runs fn inside a transaction that first deactivates every
// existing active admin, so the guarded statements see only the admins the
// subtest seeds. The transaction always rolls back, so the shared database is
// unchanged for sibling subtests.
func withAdminFixture(t *testing.T, fn func(ctx context.Context, repo *Repository)) {
	t.Helper()

	rollback := errors.New("rollback fixture")
	db := database.DB{Primary: testPool}

	err := database.NewTxRunner(testPool).Run(context.Background(), func(ctx context.Context) error {
		_, execErr := database.PrimaryDB(ctx, db).Exec(ctx,
			`UPDATE users SET active = false WHERE role = 'admin' AND active AND deleted_at IS NULL`)
		require.NoError(t, execErr)

		fn(ctx, New(db))

		return rollback
	})
	require.ErrorIs(t, err, rollback)
}

func seedAdmin(ctx context.Context, t *testing.T, active bool) uuid.UUID {
	t.Helper()

	id := uuid.New()
	_, err := database.PrimaryDB(ctx, database.DB{Primary: testPool}).Exec(ctx,
		`INSERT INTO users (id, email, password_hash, first_name, last_name, role, active)
		 VALUES ($1, $2, 'x', 'Admin', 'User', 'admin', $3)`,
		id, "admin-"+id.String()+"@example.test", active,
	)
	require.NoError(t, err)

	return id
}

// isolateActiveAdmins deactivates every active admin the subtest did not seed,
// committed, so a test that needs real concurrent transactions sees only its own
// admins. Cleanup reactivates exactly the rows it deactivated.
func isolateActiveAdmins(t *testing.T) {
	t.Helper()

	rows, err := testPool.Query(context.Background(),
		`UPDATE users SET active = false
		 WHERE role = 'admin' AND active AND deleted_at IS NULL
		 RETURNING id`)
	require.NoError(t, err)
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	require.NoError(t, err)

	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(),
			`UPDATE users SET active = true WHERE id = ANY($1)`, ids)
		require.NoError(t, err)
	})
}

func waitForLockWaiters(t *testing.T, want int) {
	t.Helper()

	require.Eventually(t, func() bool {
		var n int
		err := testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM pg_stat_activity
			 WHERE datname = current_database() AND wait_event_type = 'Lock'`,
		).Scan(&n)
		return err == nil && n >= want
	}, 5*time.Second, 10*time.Millisecond)
}
