package user_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/auth"
	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/middleware"
	mocks "github.com/residwi/go-api-project-template/mocks/user"
)

func TestService_GetByEmail(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		repo.EXPECT().GetByEmail(mock.Anything, "alice@example.com").
			Return(&user.User{
				ID:           id,
				Email:        "alice@example.com",
				PasswordHash: "hash123",
				FirstName:    "Alice",
				LastName:     "Smith",
				Role:         "user",
				Active:       true,
				TokenVersion: 1,
			}, nil)

		creds, err := svc.GetByEmail(context.Background(), "alice@example.com")
		require.NoError(t, err)
		assert.Equal(t, auth.UserCredentials{
			ID:           id,
			Email:        "alice@example.com",
			PasswordHash: "hash123",
			FirstName:    "Alice",
			LastName:     "Smith",
			Role:         "user",
			Active:       true,
			TokenVersion: 1,
		}, creds)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		repo.EXPECT().GetByEmail(mock.Anything, "nobody@example.com").
			Return(nil, core.ErrNotFound)

		_, err := svc.GetByEmail(context.Background(), "nobody@example.com")
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestService_Create(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*user.User")).
			Run(func(_ context.Context, u *user.User) {
				u.ID = uuid.New()
				u.CreatedAt = time.Now()
				u.UpdatedAt = time.Now()
			}).
			Return(nil)

		result, err := svc.Create(context.Background(), auth.CreateUserParams{
			Email:        "bob@example.com",
			PasswordHash: "hashed",
			FirstName:    "Bob",
			LastName:     "Jones",
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		assert.Equal(t, auth.UserResult{
			Email:     "bob@example.com",
			FirstName: "Bob",
			LastName:  "Jones",
			Role:      "user",
			Active:    true,
		}, result)
	})

	t.Run("conflict error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*user.User")).
			Return(core.ErrConflict)

		_, err := svc.Create(context.Background(), auth.CreateUserParams{
			Email:        "dup@example.com",
			PasswordHash: "hashed",
			FirstName:    "Dup",
			LastName:     "User",
		})
		assert.ErrorIs(t, err, core.ErrConflict)
	})
}

func TestService_GetByID(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&user.User{
				ID:           id,
				Email:        "alice@example.com",
				FirstName:    "Alice",
				LastName:     "Smith",
				Role:         "admin",
				Active:       true,
				TokenVersion: 3,
			}, nil)

		result, err := svc.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, auth.UserResult{
			ID:           id,
			Email:        "alice@example.com",
			FirstName:    "Alice",
			LastName:     "Smith",
			Role:         "admin",
			Active:       true,
			TokenVersion: 3,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, core.ErrNotFound)

		_, err := svc.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestService_CheckStatus(t *testing.T) {
	t.Run("success with no redis falls through to DB", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&user.User{
				ID:           id,
				Active:       true,
				TokenVersion: 5,
			}, nil)

		result, err := svc.CheckStatus(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, middleware.UserStatusResult{Active: true, TokenVersion: 5}, result)
	})

	t.Run("repo GetByID error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		dbErr := errors.New("database timeout")
		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, dbErr)

		_, err := svc.CheckStatus(context.Background(), uuid.New())
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_GetProfile(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		expected := &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Phone:     "555-1234",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(expected, nil)

		result, err := svc.GetProfile(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, core.ErrNotFound)

		_, err := svc.GetProfile(context.Background(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestService_UpdateProfile(t *testing.T) {
	t.Run("success partial update", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		existing := &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Phone:     "555-0000",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)

		phone := "555-9999"
		result, err := svc.UpdateProfile(context.Background(), id, user.UpdateProfileRequest{
			FirstName: "Alicia",
			Phone:     &phone,
		})
		require.NoError(t, err)
		assert.Equal(t, &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alicia",
			LastName:  "Smith",
			Phone:     "555-9999",
			Role:      "user",
			Active:    true,
		}, result)
	})

	t.Run("updates last name", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		existing := &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)

		result, err := svc.UpdateProfile(context.Background(), id, user.UpdateProfileRequest{
			LastName: "Jones",
		})
		require.NoError(t, err)
		assert.Equal(t, "Jones", result.LastName)
		assert.Equal(t, "Alice", result.FirstName)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, core.ErrNotFound)

		_, err := svc.UpdateProfile(context.Background(), uuid.New(), user.UpdateProfileRequest{FirstName: "X"})
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("repo Update error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		existing := &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)

		updateErr := errors.New("database write failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(updateErr)

		_, err := svc.UpdateProfile(context.Background(), id, user.UpdateProfileRequest{FirstName: "Alicia"})
		assert.ErrorIs(t, err, updateErr)
	})
}

func TestService_List(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		params := user.ListParams{Page: 1, PageSize: 10}
		users := []user.User{
			{ID: uuid.New(), Email: "a@example.com", FirstName: "A", LastName: "User"},
			{ID: uuid.New(), Email: "b@example.com", FirstName: "B", LastName: "User"},
		}
		repo.EXPECT().List(mock.Anything, params).Return(users, 2, nil)

		result, total, err := svc.List(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, total)
	})
}

func TestService_AdminUpdate(t *testing.T) {
	t.Run("success updates active status", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		existing := &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)

		active := false
		result, err := svc.AdminUpdate(context.Background(), id, user.AdminUpdateUserRequest{
			Active: &active,
		})
		require.NoError(t, err)
		assert.Equal(t, &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    false,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, core.ErrNotFound)

		_, err := svc.AdminUpdate(context.Background(), uuid.New(), user.AdminUpdateUserRequest{})
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("repo Update error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		existing := &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)

		updateErr := errors.New("database write failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(updateErr)

		_, err := svc.AdminUpdate(context.Background(), id, user.AdminUpdateUserRequest{FirstName: "Bob"})
		assert.ErrorIs(t, err, updateErr)
	})

	t.Run("partial update with all fields", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		id := uuid.New()
		existing := &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Phone:     "555-0000",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)

		phone := "555-9999"
		active := false
		result, err := svc.AdminUpdate(context.Background(), id, user.AdminUpdateUserRequest{
			FirstName: "Bob",
			LastName:  "Jones",
			Phone:     &phone,
			Active:    &active,
		})
		require.NoError(t, err)
		assert.Equal(t, &user.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Bob",
			LastName:  "Jones",
			Phone:     "555-9999",
			Role:      "user",
			Active:    false,
		}, result)
	})
}

func TestService_UpdateRole(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "user",
			}, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)

		err := svc.UpdateRole(context.Background(), requesterID, targetID, "admin")
		require.NoError(t, err)
	})

	t.Run("self-demotion blocked", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		sameID := uuid.New()

		err := svc.UpdateRole(context.Background(), sameID, sameID, "user")
		assert.ErrorIs(t, err, core.ErrForbidden)
	})

	t.Run("last admin blocked", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(1, nil)

		err := svc.UpdateRole(context.Background(), requesterID, targetID, "user")
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("CountAdmins error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "admin",
			}, nil)

		countErr := errors.New("count query failed")
		repo.EXPECT().CountAdmins(mock.Anything).Return(0, countErr)

		err := svc.UpdateRole(context.Background(), requesterID, targetID, "user")
		assert.ErrorIs(t, err, countErr)
	})

	t.Run("multiple admins allows demotion", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(3, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)

		err := svc.UpdateRole(context.Background(), requesterID, targetID, "user")
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, core.ErrNotFound)

		err := svc.UpdateRole(context.Background(), uuid.New(), uuid.New(), "admin")
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("Update error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "user",
			}, nil)

		updateErr := errors.New("database write failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(updateErr)

		err := svc.UpdateRole(context.Background(), requesterID, targetID, "admin")
		assert.ErrorIs(t, err, updateErr)
	})
}

func TestService_Delete(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "user",
			}, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)

		err := svc.Delete(context.Background(), requesterID, targetID)
		require.NoError(t, err)
	})

	t.Run("self-deletion blocked", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		sameID := uuid.New()

		err := svc.Delete(context.Background(), sameID, sameID)
		assert.ErrorIs(t, err, core.ErrForbidden)
	})

	t.Run("last admin blocked", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(1, nil)

		err := svc.Delete(context.Background(), requesterID, targetID)
		assert.ErrorIs(t, err, core.ErrBadRequest)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, core.ErrNotFound)

		err := svc.Delete(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, core.ErrNotFound)
	})

	t.Run("CountAdmins error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "admin",
			}, nil)

		countErr := errors.New("count query failed")
		repo.EXPECT().CountAdmins(mock.Anything).Return(0, countErr)

		err := svc.Delete(context.Background(), requesterID, targetID)
		assert.ErrorIs(t, err, countErr)
	})

	t.Run("multiple admins allows delete", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(3, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)

		err := svc.Delete(context.Background(), requesterID, targetID)
		require.NoError(t, err)
	})

	t.Run("Delete repo error propagates", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := user.NewService(repo, nil, nil)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&user.User{
				ID:   targetID,
				Role: "user",
			}, nil)

		deleteErr := errors.New("database delete failed")
		repo.EXPECT().Delete(mock.Anything, targetID).Return(deleteErr)

		err := svc.Delete(context.Background(), requesterID, targetID)
		assert.ErrorIs(t, err, deleteErr)
	})
}
