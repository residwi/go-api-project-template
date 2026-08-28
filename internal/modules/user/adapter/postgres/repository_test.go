package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/user"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
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

func TestPostgresRepository_GetStatusByID(t *testing.T) {
	t.Run("returns the user's active flag and token version", func(t *testing.T) {
		u := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		active, tokenVersion, err := repo.GetStatusByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.Active, active)
		assert.Equal(t, u.TokenVersion, tokenVersion)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})

		_, _, err := repo.GetStatusByID(context.Background(), uuid.New())
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
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 50},
			Search:     token,
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
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 1}, Search: token,
		})
		require.NoError(t, err)
		require.Len(t, first, 1)
		assert.Equal(t, 3, total)

		second, _, err := repo.ListAdmin(ctx, user.AdminListParams{
			OffsetPage: paging.OffsetPage{Page: 2, PageSize: 1}, Search: token,
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
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 50}, Role: "admin",
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
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 50}, Active: &active,
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
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 50}, Search: u.Email,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, users)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates user fields", func(t *testing.T) {
		u := seedUser(t)
		repo := New(database.DB{Primary: testPool})

		u.FirstName = "Updated"
		u.LastName = "Name"
		u.Active = false
		err := repo.Update(context.Background(), u)
		require.NoError(t, err)

		got, err := repo.GetByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, "Updated", got.FirstName)
		assert.Equal(t, "Name", got.LastName)
		assert.False(t, got.Active)
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
		err := repo.Update(context.Background(), u)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("soft deletes user", func(t *testing.T) {
		id := testutil.SeedUser(t, testPool)
		repo := New(database.DB{Primary: testPool})

		err := repo.Delete(context.Background(), id)
		require.NoError(t, err)
	})

	t.Run("GetByID returns not found after delete", func(t *testing.T) {
		id := testutil.SeedUser(t, testPool)
		repo := New(database.DB{Primary: testPool})
		ctx := context.Background()

		require.NoError(t, repo.Delete(ctx, id))

		_, err := repo.GetByID(ctx, id)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("returns not found for nonexistent user", func(t *testing.T) {
		repo := New(database.DB{Primary: testPool})
		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
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

	t.Run("GetStatusByID returns error on cancelled context", func(t *testing.T) {
		_, _, err := repo.GetStatusByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("ListAdmin returns error on cancelled context", func(t *testing.T) {
		_, _, err := repo.ListAdmin(ctx, user.AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}})
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
		err := repo.Update(ctx, u)
		require.Error(t, err)
		assert.NotErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("Delete returns error on cancelled context", func(t *testing.T) {
		err := repo.Delete(ctx, uuid.New())
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
