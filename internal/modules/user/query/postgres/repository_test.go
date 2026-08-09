package postgres

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/user/domain"
	"github.com/residwi/go-api-project-template/internal/modules/user/query"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup := testhelper.MustStartPostgres("test_user")
	defer cleanup()
	testPool = pool
	os.Exit(m.Run())
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns user", func(t *testing.T) {
		u := seedUser(t)
		repo := New(testPool)

		got, err := repo.GetByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.ID, got.ID)
		assert.Equal(t, u.Email, got.Email)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_GetStatusByID(t *testing.T) {
	t.Run("returns the user's active flag and token version", func(t *testing.T) {
		u := seedUser(t)
		repo := New(testPool)

		active, tokenVersion, err := repo.GetStatusByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.Active, active)
		assert.Equal(t, u.TokenVersion, tokenVersion)
	})

	t.Run("returns not found", func(t *testing.T) {
		repo := New(testPool)

		_, _, err := repo.GetStatusByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestPostgresRepository_ListAdmin(t *testing.T) {
	t.Run("returns the users this subtest seeded", func(t *testing.T) {
		token := "listadmin-" + uuid.New().String()[:8]
		u1 := seedUserWithEmailToken(t, token)
		u2 := seedUserWithEmailToken(t, token)
		repo := New(testPool)

		users, total, err := repo.ListAdmin(context.Background(), query.Params{
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
		repo := New(testPool)
		ctx := context.Background()

		first, total, err := repo.ListAdmin(ctx, query.Params{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 1}, Search: token,
		})
		require.NoError(t, err)
		require.Len(t, first, 1)
		assert.Equal(t, 3, total)

		second, _, err := repo.ListAdmin(ctx, query.Params{
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
		repo := New(testPool)

		users, total, err := repo.ListAdmin(context.Background(), query.Params{
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
		repo := New(testPool)
		active := true

		users, _, err := repo.ListAdmin(context.Background(), query.Params{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 50}, Active: &active,
		})
		require.NoError(t, err)
		for _, u := range users {
			assert.True(t, u.Active)
		}
	})

	t.Run("filters by search", func(t *testing.T) {
		u := seedUser(t)
		repo := New(testPool)

		users, total, err := repo.ListAdmin(context.Background(), query.Params{
			OffsetPage: paging.OffsetPage{Page: 1, PageSize: 50}, Search: u.Email,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, users)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := New(testPool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("GetByID returns error on cancelled context", func(t *testing.T) {
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("GetStatusByID returns error on cancelled context", func(t *testing.T) {
		_, _, err := repo.GetStatusByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("ListAdmin returns error on cancelled context", func(t *testing.T) {
		_, _, err := repo.ListAdmin(ctx, query.Params{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}})
		require.Error(t, err)
	})
}

func seedUser(t *testing.T) *domain.User {
	t.Helper()
	id := testhelper.SeedUser(t, testPool)

	repo := New(testPool)
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

	repo := New(testPool)
	u, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return u
}
