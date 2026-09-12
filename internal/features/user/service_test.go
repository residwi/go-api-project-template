package user

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/user/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

func TestService_GetByEmail(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		repo.EXPECT().GetByEmail(mock.Anything, "alice@example.com").
			Return(&domain.User{
				ID:           id,
				Email:        "alice@example.com",
				PasswordHash: "hash123",
				FirstName:    "Alice",
				LastName:     "Smith",
				Role:         "user",
				Active:       true,
				TokenVersion: 1,
			}, nil)

		creds, err := s.GetByEmail(context.Background(), "alice@example.com")
		require.NoError(t, err)
		assert.Equal(t, Credentials{
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
		s := New(repo)

		repo.EXPECT().GetByEmail(mock.Anything, "nobody@example.com").
			Return(nil, errs.ErrNotFound)

		_, err := s.GetByEmail(context.Background(), "nobody@example.com")
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestService_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.User")).
			Run(func(_ context.Context, u *domain.User) {
				u.ID = uuid.New()
			}).
			Return(nil)

		result, err := s.Create(context.Background(), NewUser{
			Email:        "bob@example.com",
			PasswordHash: "hashed",
			FirstName:    "Bob",
			LastName:     "Jones",
		})
		require.NoError(t, err)
		assert.NotEqual(t, uuid.Nil, result.ID)
		result.ID = uuid.Nil
		assert.Equal(t, Profile{
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
		s := New(repo)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*domain.User")).
			Return(errs.ErrConflict)

		_, err := s.Create(context.Background(), NewUser{
			Email:        "dup@example.com",
			PasswordHash: "hashed",
			FirstName:    "Dup",
			LastName:     "User",
		})
		assert.ErrorIs(t, err, errs.ErrConflict)
	})
}

func TestService_GetByID(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).
			Return(&domain.User{
				ID:           id,
				Email:        "alice@example.com",
				FirstName:    "Alice",
				LastName:     "Smith",
				Role:         "admin",
				Active:       true,
				TokenVersion: 3,
			}, nil)

		result, err := s.GetByID(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, Profile{
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
		s := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		_, err := s.GetByID(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

// GetUser serves both /users/me and the admin /users/{id} route: the old
// query slice had already merged these from two identically-bodied methods
// before this flatten. It stays a distinct method from GetByID rather than
// folding into it, because GetByID's signature is pinned by auth's
// UserDirectory port, which this flatten must not rename.
func TestService_GetUser(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		expected := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Phone:     "555-1234",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(expected, nil)

		result, err := s.GetUser(context.Background(), id)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		_, err := s.GetUser(context.Background(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})
}

func TestService_ListAdmin(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		params := AdminListParams{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 10}}
		users := []domain.User{
			{ID: uuid.New(), Email: "a@example.com", FirstName: "A", LastName: "User"},
			{ID: uuid.New(), Email: "b@example.com", FirstName: "B", LastName: "User"},
		}
		repo.EXPECT().ListAdmin(mock.Anything, params).Return(users, 2, nil)

		result, total, err := s.ListAdmin(context.Background(), params)
		require.NoError(t, err)
		assert.Len(t, result, 2)
		assert.Equal(t, 2, total)
	})
}

func TestService_CheckStatus(t *testing.T) {
	t.Parallel()

	userID := uuid.New()

	t.Run("returns the status the repository reports", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(true, 7, nil)
		s := New(repo)

		got, err := s.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, AccountStatus{Active: true, TokenVersion: 7}, got)
	})

	t.Run("reports an inactive user as inactive", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(false, 4, nil)
		s := New(repo)

		got, err := s.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, AccountStatus{Active: false, TokenVersion: 4}, got)
	})

	t.Run("reports an unknown user as inactive rather than an error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(false, 0, errs.ErrNotFound)
		s := New(repo)

		got, err := s.CheckStatus(context.Background(), userID)

		require.NoError(t, err)
		assert.Equal(t, AccountStatus{Active: false}, got)
	})

	t.Run("propagates any other repository error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		dbErr := errors.New("database timeout")
		repo.EXPECT().GetStatusByID(mock.Anything, userID).Return(false, 0, dbErr)
		s := New(repo)

		_, err := s.CheckStatus(context.Background(), userID)

		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_UpdateProfile(t *testing.T) {
	t.Parallel()

	t.Run("success partial update", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		existing := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Phone:     "555-0000",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)

		phone := "555-9999"
		result, err := s.UpdateProfile(context.Background(), id, "Alicia", "", &phone)
		require.NoError(t, err)
		assert.Equal(t, &domain.User{
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
		s := New(repo)

		id := uuid.New()
		existing := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)

		result, err := s.UpdateProfile(context.Background(), id, "", "Jones", nil)
		require.NoError(t, err)
		assert.Equal(t, "Jones", result.LastName)
		assert.Equal(t, "Alice", result.FirstName)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		_, err := s.UpdateProfile(context.Background(), uuid.New(), "X", "", nil)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("repo Update error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		existing := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)

		updateErr := errors.New("database write failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(updateErr)

		_, err := s.UpdateProfile(context.Background(), id, "Alicia", "", nil)
		assert.ErrorIs(t, err, updateErr)
	})
}

func TestService_AdminUpdate(t *testing.T) {
	t.Parallel()

	t.Run("success updates active status", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		existing := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)

		active := false
		result, err := s.AdminUpdate(context.Background(), id, "", "", nil, &active)
		require.NoError(t, err)
		assert.Equal(t, &domain.User{
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
		s := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		_, err := s.AdminUpdate(context.Background(), uuid.New(), "", "", nil, nil)
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("repo Update error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		existing := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)

		updateErr := errors.New("database write failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(updateErr)

		_, err := s.AdminUpdate(context.Background(), id, "Bob", "", nil, nil)
		assert.ErrorIs(t, err, updateErr)
	})

	t.Run("partial update with all fields", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		id := uuid.New()
		existing := &domain.User{
			ID:        id,
			Email:     "alice@example.com",
			FirstName: "Alice",
			LastName:  "Smith",
			Phone:     "555-0000",
			Role:      "user",
			Active:    true,
		}
		repo.EXPECT().GetByID(mock.Anything, id).Return(existing, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)

		phone := "555-9999"
		active := false
		result, err := s.AdminUpdate(context.Background(), id, "Bob", "Jones", &phone, &active)
		require.NoError(t, err)
		assert.Equal(t, &domain.User{
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
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "user"}, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(nil)

		err := s.UpdateRole(context.Background(), requesterID, targetID, "admin")
		require.NoError(t, err)
	})

	t.Run("self-demotion blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		sameID := uuid.New()

		err := s.UpdateRole(context.Background(), sameID, sameID, "user")
		assert.ErrorIs(t, err, errs.ErrForbidden)
	})

	t.Run("last admin blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "admin"}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(1, nil)

		err := s.UpdateRole(context.Background(), requesterID, targetID, "user")
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("CountAdmins error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "admin"}, nil)

		countErr := errors.New("count query failed")
		repo.EXPECT().CountAdmins(mock.Anything).Return(0, countErr)

		err := s.UpdateRole(context.Background(), requesterID, targetID, "user")
		assert.ErrorIs(t, err, countErr)
	})

	t.Run("multiple admins allows demotion", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "admin"}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(3, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(nil)

		err := s.UpdateRole(context.Background(), requesterID, targetID, "user")
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		err := s.UpdateRole(context.Background(), uuid.New(), uuid.New(), "admin")
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("Update error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "user"}, nil)

		updateErr := errors.New("database write failed")
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(updateErr)

		err := s.UpdateRole(context.Background(), requesterID, targetID, "admin")
		assert.ErrorIs(t, err, updateErr)
	})

	t.Run("IncrementTokenVersion error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "user"}, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*domain.User")).Return(nil)

		incrErr := errors.New("token bump failed")
		repo.EXPECT().IncrementTokenVersion(mock.Anything, targetID).Return(incrErr)

		err := s.UpdateRole(context.Background(), requesterID, targetID, "admin")
		assert.ErrorIs(t, err, incrErr)
	})
}

func TestService_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "user"}, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)

		err := s.Delete(context.Background(), requesterID, targetID)
		require.NoError(t, err)
	})

	t.Run("self-deletion blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		sameID := uuid.New()

		err := s.Delete(context.Background(), sameID, sameID)
		assert.ErrorIs(t, err, errs.ErrForbidden)
	})

	t.Run("last admin blocked", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "admin"}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(1, nil)

		err := s.Delete(context.Background(), requesterID, targetID)
		assert.ErrorIs(t, err, errs.ErrBadRequest)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		repo.EXPECT().GetByID(mock.Anything, mock.AnythingOfType("uuid.UUID")).
			Return(nil, errs.ErrNotFound)

		err := s.Delete(context.Background(), uuid.New(), uuid.New())
		assert.ErrorIs(t, err, errs.ErrNotFound)
	})

	t.Run("CountAdmins error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "admin"}, nil)

		countErr := errors.New("count query failed")
		repo.EXPECT().CountAdmins(mock.Anything).Return(0, countErr)

		err := s.Delete(context.Background(), requesterID, targetID)
		assert.ErrorIs(t, err, countErr)
	})

	t.Run("multiple admins allows delete", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "admin"}, nil)
		repo.EXPECT().CountAdmins(mock.Anything).Return(3, nil)
		repo.EXPECT().Delete(mock.Anything, targetID).Return(nil)

		err := s.Delete(context.Background(), requesterID, targetID)
		require.NoError(t, err)
	})

	t.Run("Delete repo error propagates", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		s := New(repo)

		requesterID := uuid.New()
		targetID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, targetID).
			Return(&domain.User{ID: targetID, Role: "user"}, nil)

		deleteErr := errors.New("database delete failed")
		repo.EXPECT().Delete(mock.Anything, targetID).Return(deleteErr)

		err := s.Delete(context.Background(), requesterID, targetID)
		assert.ErrorIs(t, err, deleteErr)
	})
}
