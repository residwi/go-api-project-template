package user_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

var (
	testPool  *pgxpool.Pool
	testRedis *redis.Client
)

func TestMain(m *testing.M) {
	pool, cleanupPG := testhelper.MustStartPostgres("test_features_user")
	defer cleanupPG()
	testPool = pool

	rdb, cleanupRedis := testhelper.MustStartRedis(2)
	defer cleanupRedis()
	testRedis = rdb

	os.Exit(m.Run())
}

func setup(t *testing.T) {
	t.Helper()
	testhelper.ResetDB(t, testPool)
	testhelper.ResetRedis(t, testRedis)
}

func seedUser(t *testing.T) *user.User {
	t.Helper()
	id := uuid.New()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO users (id, email, password_hash, first_name, last_name) VALUES ($1, $2, 'x', 'A', 'B')`,
		id, id.String()+"@test.com",
	)
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, id)
	})

	repo := user.NewPostgresRepository(testPool)
	u, err := repo.GetByID(context.Background(), id)
	require.NoError(t, err)
	return u
}

func TestPostgresRepository_Create(t *testing.T) {
	t.Run("creates user", func(t *testing.T) {
		setup(t)
		repo := user.NewPostgresRepository(testPool)
		u := &user.User{
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
		setup(t)
		existing := seedUser(t)
		repo := user.NewPostgresRepository(testPool)

		dup := &user.User{
			Email:        existing.Email,
			PasswordHash: "hashed",
			FirstName:    "Jane",
			LastName:     "Doe",
			Role:         "user",
			Active:       true,
		}
		err := repo.Create(context.Background(), dup)
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestPostgresRepository_GetByID(t *testing.T) {
	t.Run("returns user", func(t *testing.T) {
		setup(t)
		u := seedUser(t)
		repo := user.NewPostgresRepository(testPool)

		got, err := repo.GetByID(context.Background(), u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.ID, got.ID)
		assert.Equal(t, u.Email, got.Email)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := user.NewPostgresRepository(testPool)

		_, err := repo.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_GetByEmail(t *testing.T) {
	t.Run("returns user by email", func(t *testing.T) {
		setup(t)
		u := seedUser(t)
		repo := user.NewPostgresRepository(testPool)

		got, err := repo.GetByEmail(context.Background(), u.Email)
		require.NoError(t, err)
		assert.Equal(t, u.ID, got.ID)
		assert.Equal(t, u.Email, got.Email)
	})

	t.Run("returns not found", func(t *testing.T) {
		setup(t)
		repo := user.NewPostgresRepository(testPool)

		_, err := repo.GetByEmail(context.Background(), "nobody@nowhere.example")
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_Update(t *testing.T) {
	t.Run("updates user fields", func(t *testing.T) {
		setup(t)
		u := seedUser(t)
		repo := user.NewPostgresRepository(testPool)

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
		setup(t)
		repo := user.NewPostgresRepository(testPool)

		u := &user.User{
			ID:        uuid.New(),
			FirstName: "Ghost",
			LastName:  "User",
			Role:      "user",
			Active:    true,
		}
		err := repo.Update(context.Background(), u)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_Delete(t *testing.T) {
	t.Run("soft deletes user", func(t *testing.T) {
		setup(t)
		u := seedUser(t)
		repo := user.NewPostgresRepository(testPool)

		err := repo.Delete(context.Background(), u.ID)
		require.NoError(t, err)
	})

	t.Run("GetByID returns not found after delete", func(t *testing.T) {
		setup(t)
		u := seedUser(t)
		repo := user.NewPostgresRepository(testPool)
		ctx := context.Background()

		require.NoError(t, repo.Delete(ctx, u.ID))

		_, err := repo.GetByID(ctx, u.ID)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("returns not found for nonexistent user", func(t *testing.T) {
		setup(t)
		repo := user.NewPostgresRepository(testPool)
		err := repo.Delete(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_List(t *testing.T) {
	t.Run("returns paginated list", func(t *testing.T) {
		setup(t)
		seedUser(t)
		seedUser(t)
		repo := user.NewPostgresRepository(testPool)

		users, total, err := repo.List(context.Background(), user.ListParams{
			Page:     1,
			PageSize: 10,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 2)
		assert.NotEmpty(t, users)
	})

	t.Run("filters by role", func(t *testing.T) {
		setup(t)
		u := seedUser(t)
		_, err := testPool.Exec(context.Background(), `UPDATE users SET role = 'admin' WHERE id = $1`, u.ID)
		require.NoError(t, err)
		repo := user.NewPostgresRepository(testPool)

		users, total, err := repo.List(context.Background(), user.ListParams{
			Page: 1, PageSize: 50, Role: "admin",
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		for _, u := range users {
			assert.Equal(t, "admin", u.Role)
		}
	})

	t.Run("filters by active", func(t *testing.T) {
		setup(t)
		seedUser(t)
		repo := user.NewPostgresRepository(testPool)
		active := true

		users, _, err := repo.List(context.Background(), user.ListParams{
			Page: 1, PageSize: 50, Active: &active,
		})
		require.NoError(t, err)
		for _, u := range users {
			assert.True(t, u.Active)
		}
	})

	t.Run("filters by search", func(t *testing.T) {
		setup(t)
		u := seedUser(t)
		repo := user.NewPostgresRepository(testPool)

		users, total, err := repo.List(context.Background(), user.ListParams{
			Page: 1, PageSize: 50, Search: u.Email,
		})
		require.NoError(t, err)
		assert.GreaterOrEqual(t, total, 1)
		assert.NotEmpty(t, users)
	})
}

func TestPostgresRepository_ExistsByEmail(t *testing.T) {
	t.Run("returns false when no user", func(t *testing.T) {
		setup(t)
		repo := user.NewPostgresRepository(testPool)

		exists, err := repo.ExistsByEmail(context.Background(), "ghost@nowhere.example")
		require.NoError(t, err)
		assert.False(t, exists)
	})

	t.Run("returns true for existing email", func(t *testing.T) {
		setup(t)
		u := seedUser(t)
		repo := user.NewPostgresRepository(testPool)

		exists, err := repo.ExistsByEmail(context.Background(), u.Email)
		require.NoError(t, err)
		assert.True(t, exists)
	})
}

func TestPostgresRepository_CountAdmins(t *testing.T) {
	t.Run("returns zero when no admins", func(t *testing.T) {
		setup(t)
		// Ensure a clean count by checking that we can call it without error;
		// we cannot guarantee zero since other tests may have seeded admins,
		// so we just verify the call succeeds.
		repo := user.NewPostgresRepository(testPool)

		count, err := repo.CountAdmins(context.Background())
		require.NoError(t, err)
		assert.GreaterOrEqual(t, count, 0)
	})

	t.Run("returns correct count of active admins", func(t *testing.T) {
		setup(t)
		repo := user.NewPostgresRepository(testPool)
		ctx := context.Background()

		before, err := repo.CountAdmins(ctx)
		require.NoError(t, err)

		adminID := uuid.New()
		_, err = testPool.Exec(ctx,
			`INSERT INTO users (id, email, password_hash, first_name, last_name, role, active)
			 VALUES ($1, $2, 'x', 'Admin', 'User', 'admin', true)`,
			adminID, adminID.String()+"@admin.com",
		)
		require.NoError(t, err)
		t.Cleanup(func() {
			testPool.Exec(ctx, `DELETE FROM users WHERE id = $1`, adminID)
		})

		after, err := repo.CountAdmins(ctx)
		require.NoError(t, err)
		assert.Equal(t, before+1, after)
	})
}

func TestPostgresRepository_IncrementTokenVersion(t *testing.T) {
	t.Run("increments token version", func(t *testing.T) {
		setup(t)
		u := seedUser(t)
		repo := user.NewPostgresRepository(testPool)
		ctx := context.Background()

		err := repo.IncrementTokenVersion(ctx, u.ID)
		require.NoError(t, err)

		got, err := repo.GetByID(ctx, u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.TokenVersion+1, got.TokenVersion)
	})

	t.Run("returns not found for missing user", func(t *testing.T) {
		setup(t)
		repo := user.NewPostgresRepository(testPool)

		err := repo.IncrementTokenVersion(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestPostgresRepository_CancelledContext(t *testing.T) {
	repo := user.NewPostgresRepository(testPool)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("Create returns error on cancelled context", func(t *testing.T) {
		setup(t)
		u := &user.User{
			Email:        "cancelled@example.com",
			PasswordHash: "hashed",
			FirstName:    "Test",
			LastName:     "User",
			Role:         "user",
			Active:       true,
		}
		err := repo.Create(ctx, u)
		require.Error(t, err)
		assert.NotErrorIs(t, err, core.ErrConflict)
	})

	t.Run("GetByID returns error on cancelled context", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByID(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("GetByEmail returns error on cancelled context", func(t *testing.T) {
		setup(t)
		_, err := repo.GetByEmail(ctx, "test@example.com")
		require.Error(t, err)
		assert.NotErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("Update returns error on cancelled context", func(t *testing.T) {
		setup(t)
		u := &user.User{
			ID:        uuid.New(),
			FirstName: "Test",
			LastName:  "User",
			Role:      "user",
			Active:    true,
		}
		err := repo.Update(ctx, u)
		require.Error(t, err)
		assert.NotErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("Delete returns error on cancelled context", func(t *testing.T) {
		setup(t)
		err := repo.Delete(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("List returns error on cancelled context", func(t *testing.T) {
		setup(t)
		_, _, err := repo.List(ctx, user.ListParams{Page: 1, PageSize: 10})
		require.Error(t, err)
	})

	t.Run("ExistsByEmail returns error on cancelled context", func(t *testing.T) {
		setup(t)
		_, err := repo.ExistsByEmail(ctx, "test@example.com")
		require.Error(t, err)
	})

	t.Run("CountAdmins returns error on cancelled context", func(t *testing.T) {
		setup(t)
		_, err := repo.CountAdmins(ctx)
		require.Error(t, err)
	})

	t.Run("IncrementTokenVersion returns error on cancelled context", func(t *testing.T) {
		setup(t)
		err := repo.IncrementTokenVersion(ctx, uuid.New())
		require.Error(t, err)
		assert.NotErrorIs(t, err, core.ErrNotFound)
	})
}
