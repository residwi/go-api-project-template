package login

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		tokens := NewMockTokenIssuer(t)

		userID := uuid.New()
		hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)
		creds := usercontract.Credentials{
			ID:           userID,
			Email:        "test@example.com",
			PasswordHash: string(hash),
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "customer",
			Active:       true,
			TokenVersion: 1,
		}

		users.EXPECT().GetByEmail(mock.Anything, "test@example.com").Return(creds, nil)
		tokens.EXPECT().BuildTokenPair(usercontract.User{
			ID:           userID,
			Email:        "test@example.com",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "customer",
			Active:       true,
			TokenVersion: 1,
		}).Return(&domain.TokenPair{AccessToken: "access-token", RefreshToken: "refresh-token"}, nil)

		resp, err := New(users, tokens, bcrypt.MinCost).Execute(context.Background(), Params{
			Email:    "test@example.com",
			Password: "password123",
		})

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
	})

	t.Run("inactive user returns ErrUnauthorized", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		tokens := NewMockTokenIssuer(t)

		hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)

		users.EXPECT().GetByEmail(mock.Anything, "inactive@example.com").Return(usercontract.Credentials{
			ID:           uuid.New(),
			Email:        "inactive@example.com",
			PasswordHash: string(hash),
			Active:       false,
		}, nil)

		resp, err := New(users, tokens, bcrypt.MinCost).Execute(context.Background(), Params{
			Email:    "inactive@example.com",
			Password: "password123",
		})

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrUnauthorized)
	})

	t.Run("wrong password returns ErrInvalidCredentials", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		tokens := NewMockTokenIssuer(t)

		hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)

		users.EXPECT().GetByEmail(mock.Anything, "test@example.com").Return(usercontract.Credentials{
			ID:           uuid.New(),
			Email:        "test@example.com",
			PasswordHash: string(hash),
			Active:       true,
		}, nil)

		resp, err := New(users, tokens, bcrypt.MinCost).Execute(context.Background(), Params{
			Email:    "test@example.com",
			Password: "wrong-password",
		})

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrInvalidCredentials)
	})

	t.Run("user not found returns ErrInvalidCredentials", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		tokens := NewMockTokenIssuer(t)

		users.EXPECT().
			GetByEmail(mock.Anything, "notfound@example.com").
			Return(usercontract.Credentials{}, errors.New("not found"))

		resp, err := New(users, tokens, bcrypt.MinCost).Execute(context.Background(), Params{
			Email:    "notfound@example.com",
			Password: "password123",
		})

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrInvalidCredentials)
	})
}
