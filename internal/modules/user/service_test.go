package user

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/auth"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestService_CheckStatus(t *testing.T) {
	t.Parallel()

	userID := uuid.New()

	t.Run("returns the cached snapshot without touching the repository", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).
			Return(StatusSnapshot{Active: true, TokenVersion: 42}, true, nil)
		svc := NewService(repo, c, testhelper.DiscardLogger())

		got, err := svc.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, middleware.UserStatusResult{Active: true, TokenVersion: 42}, got)
	})

	t.Run("reads the repository on a miss and writes the snapshot back", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(true, 7, nil)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).Return(StatusSnapshot{}, false, nil)
		c.EXPECT().Put(mock.Anything, userID,
			StatusSnapshot{Active: true, TokenVersion: 7}, 30*time.Second).Return(nil)
		svc := NewService(repo, c, testhelper.DiscardLogger())

		got, err := svc.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, middleware.UserStatusResult{Active: true, TokenVersion: 7}, got)
	})

	t.Run("falls back to the repository when the cache read errors", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(true, 3, nil)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).
			Return(StatusSnapshot{}, false, errors.New("backend down"))
		c.EXPECT().Put(mock.Anything, userID, mock.Anything, mock.Anything).Return(nil)
		svc := NewService(repo, c, testhelper.DiscardLogger())

		got, err := svc.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, middleware.UserStatusResult{Active: true, TokenVersion: 3}, got)
	})

	t.Run("caches an inactive user as inactive", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(false, 4, nil)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).Return(StatusSnapshot{}, false, nil)
		c.EXPECT().Put(mock.Anything, userID,
			StatusSnapshot{Active: false, TokenVersion: 4}, 30*time.Second).Return(nil)
		svc := NewService(repo, c, testhelper.DiscardLogger())

		got, err := svc.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, middleware.UserStatusResult{Active: false, TokenVersion: 4}, got)
	})

	t.Run("reports a deleted user as inactive rather than an error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(false, 0, apperror.ErrNotFound)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).Return(StatusSnapshot{}, false, nil)
		svc := NewService(repo, c, testhelper.DiscardLogger())

		got, err := svc.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, middleware.UserStatusResult{Active: false, TokenVersion: 0}, got)
	})

	t.Run("still returns the result when the cache write fails", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(true, 1, nil)
		c := NewMockStatusCache(t)
		c.EXPECT().Get(mock.Anything, userID).Return(StatusSnapshot{}, false, nil)
		c.EXPECT().Put(mock.Anything, userID, mock.Anything, mock.Anything).
			Return(errors.New("backend down"))
		svc := NewService(repo, c, testhelper.DiscardLogger())

		got, err := svc.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, middleware.UserStatusResult{Active: true, TokenVersion: 1}, got)
	})

	t.Run("works with NoCache, always reading through to the repository", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(true, 9, nil)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		got, err := svc.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, middleware.UserStatusResult{Active: true, TokenVersion: 9}, got)
	})
}

func TestService_GetByEmail(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		repo.EXPECT().GetByEmail(mock.Anything, "alice@example.com").
			Return(&User{
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
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().GetByEmail(mock.Anything, "nobody@example.com").
			Return(nil, apperror.ErrNotFound)

		_, err := svc.GetByEmail(context.Background(), "nobody@example.com")
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*user.User")).
			Run(func(_ context.Context, u *User) {
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
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*user.User")).
			Return(apperror.ErrConflict)

		_, err := svc.Create(context.Background(), auth.CreateUserParams{
			Email:        "dup@example.com",
			PasswordHash: "hashed",
			FirstName:    "Dup",
			LastName:     "User",
		})
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})
}

func TestService_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&User{
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
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := svc.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_CheckStatus_RepoErrorPropagates(t *testing.T) {
	t.Parallel()

	t.Run("repo GetStatusByID error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		dbErr := errors.New("database timeout")
		repo.EXPECT().GetStatusByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(false, 0, dbErr)

		_, err := svc.CheckStatus(context.Background(), uuid.New())
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_GetProfile(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		expected := &User{
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
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := svc.GetProfile(context.Background(), uuid.New())
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})
}

func TestService_UpdateProfile(t *testing.T) {
	t.Parallel()

	t.Run("success partial update", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		existing := &User{
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
		result, err := svc.UpdateProfile(context.Background(), id, UpdateProfileParams{
			FirstName: "Alicia",
			Phone:     &phone,
		})
		require.NoError(t, err)
		assert.Equal(t, &User{
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
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		existing := &User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)

		result, err := svc.UpdateProfile(context.Background(), id, UpdateProfileParams{
			LastName: "Jones",
		})
		require.NoError(t, err)
		assert.Equal(t, "Jones", result.LastName)
		assert.Equal(t, "Alice", result.FirstName)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := svc.UpdateProfile(context.Background(), uuid.New(), UpdateProfileParams{FirstName: "X"})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repo Update error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		existing := &User{
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

		_, err := svc.UpdateProfile(context.Background(), id, UpdateProfileParams{FirstName: "Alicia"})
		assert.ErrorIs(t, err, updateErr)
	})
}

func TestService_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		params := ListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}}
		users := []User{
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
	t.Parallel()

	t.Run("success updates active status", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		existing := &User{
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
		result, err := svc.AdminUpdate(context.Background(), id, AdminUpdateParams{
			Active: &active,
		})
		require.NoError(t, err)
		assert.Equal(t, &User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    false,
		}, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		_, err := svc.AdminUpdate(context.Background(), uuid.New(), AdminUpdateParams{})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("repo Update error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		existing := &User{
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

		_, err := svc.AdminUpdate(context.Background(), id, AdminUpdateParams{FirstName: "Bob"})
		assert.ErrorIs(t, err, updateErr)
	})

	t.Run("partial update with all fields", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		id := uuid.New()
		existing := &User{
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
		result, err := svc.AdminUpdate(context.Background(), id, AdminUpdateParams{
			FirstName: "Bob",
			LastName:  "Jones",
			Phone:     &phone,
			Active:    &active,
		})
		require.NoError(t, err)
		assert.Equal(t, &User{
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
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "user",
			}, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(nil)

		err := svc.UpdateRole(context.Background(), UpdateRoleParams{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "admin",
		})
		require.NoError(t, err)
	})

	t.Run("self-demotion blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		sameID := uuid.New()

		err := svc.UpdateRole(context.Background(), UpdateRoleParams{
			RequesterID: sameID,
			TargetID:    sameID,
			Role:        "user",
		})
		assert.ErrorIs(t, err, apperror.ErrForbidden)
	})

	t.Run("last admin blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(1, nil)

		err := svc.UpdateRole(context.Background(), UpdateRoleParams{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "user",
		})
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("CountAdmins error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "admin",
			}, nil)

		countErr := errors.New("count query failed")
		repo.EXPECT().CountAdmins(mock.Anything).Return(0, countErr)

		err := svc.UpdateRole(context.Background(), UpdateRoleParams{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "user",
		})
		assert.ErrorIs(t, err, countErr)
	})

	t.Run("multiple admins allows demotion", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(3, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(nil)
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(nil)

		err := svc.UpdateRole(context.Background(), UpdateRoleParams{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "user",
		})
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		err := svc.UpdateRole(context.Background(), UpdateRoleParams{
			RequesterID: uuid.New(),
			TargetID:    uuid.New(),
			Role:        "admin",
		})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("Update error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "user",
			}, nil)

		updateErr := errors.New("database write failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*user.User")).Return(updateErr)

		err := svc.UpdateRole(context.Background(), UpdateRoleParams{
			RequesterID: requesterID,
			TargetID:    targetID,
			Role:        "admin",
		})
		assert.ErrorIs(t, err, updateErr)
	})
}

func TestService_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "user",
			}, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)

		err := svc.Delete(context.Background(), DeleteParams{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		require.NoError(t, err)
	})

	t.Run("self-deletion blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		sameID := uuid.New()

		err := svc.Delete(context.Background(), DeleteParams{
			RequesterID: sameID,
			TargetID:    sameID,
		})
		assert.ErrorIs(t, err, apperror.ErrForbidden)
	})

	t.Run("last admin blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(1, nil)

		err := svc.Delete(context.Background(), DeleteParams{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, apperror.ErrNotFound)

		err := svc.Delete(context.Background(), DeleteParams{
			RequesterID: uuid.New(),
			TargetID:    uuid.New(),
		})
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("CountAdmins error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "admin",
			}, nil)

		countErr := errors.New("count query failed")
		repo.EXPECT().CountAdmins(mock.Anything).Return(0, countErr)

		err := svc.Delete(context.Background(), DeleteParams{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		assert.ErrorIs(t, err, countErr)
	})

	t.Run("multiple admins allows delete", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "admin",
			}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(3, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)

		err := svc.Delete(context.Background(), DeleteParams{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		require.NoError(t, err)
	})

	t.Run("Delete repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&User{
				ID:   targetID,
				Role: "user",
			}, nil)

		deleteErr := errors.New("database delete failed")
		repo.EXPECT().Delete(mock.Anything, targetID).Return(deleteErr)

		err := svc.Delete(context.Background(), DeleteParams{
			RequesterID: requesterID,
			TargetID:    targetID,
		})
		assert.ErrorIs(t, err, deleteErr)
	})
}

func TestService_Delete_RejectsSelfDeleteByName(t *testing.T) {
	t.Parallel()

	repo := NewMockRepository(t)
	svc := NewService(repo, NoCache{}, testhelper.DiscardLogger())

	actorID := uuid.New()

	err := svc.Delete(context.Background(), DeleteParams{
		RequesterID: actorID,
		TargetID:    actorID,
	})
	require.ErrorIs(t, err, apperror.ErrForbidden)
}
