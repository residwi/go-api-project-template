package refresh

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	authcontract "github.com/residwi/go-api-project-template/internal/modules/auth/contract"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		validate := NewMockTokenValidator(t)
		tokens := NewMockTokenIssuer(t)

		userID := uuid.New()
		validate.EXPECT().ValidateToken("valid-refresh-token").Return(authcontract.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			Type:         "refresh",
			TokenVersion: 1,
		}, nil)

		user := usercontract.User{
			ID:           userID,
			Email:        "test@example.com",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "customer",
			Active:       true,
			TokenVersion: 1,
		}
		users.EXPECT().GetByID(mock.Anything, userID).Return(user, nil)
		tokens.EXPECT().BuildTokenPair(user).Return(&domain.TokenPair{
			AccessToken:  "new-access-token",
			RefreshToken: "new-refresh-token",
		}, nil)

		resp, err := New(users, validate, tokens).Execute(context.Background(), "valid-refresh-token")

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
	})

	t.Run("invalid token returns ErrInvalidToken", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		validate := NewMockTokenValidator(t)
		tokens := NewMockTokenIssuer(t)

		validate.EXPECT().ValidateToken("invalid-token").
			Return(authcontract.Claims{}, errors.New("invalid token claims"))

		resp, err := New(users, validate, tokens).Execute(context.Background(), "invalid-token")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrInvalidToken)
	})

	t.Run("access token instead of refresh returns ErrInvalidToken", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		validate := NewMockTokenValidator(t)
		tokens := NewMockTokenIssuer(t)

		validate.EXPECT().ValidateToken("access-token").Return(authcontract.Claims{
			UserID:       uuid.New(),
			Email:        "test@example.com",
			Role:         "customer",
			Type:         "access",
			TokenVersion: 1,
		}, nil)

		resp, err := New(users, validate, tokens).Execute(context.Background(), "access-token")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrInvalidToken)
	})

	t.Run("inactive user returns ErrUnauthorized", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		validate := NewMockTokenValidator(t)
		tokens := NewMockTokenIssuer(t)

		userID := uuid.New()
		validate.EXPECT().ValidateToken("refresh-token").Return(authcontract.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			Type:         "refresh",
			TokenVersion: 1,
		}, nil)

		users.EXPECT().GetByID(mock.Anything, userID).Return(usercontract.User{
			ID:           userID,
			Email:        "test@example.com",
			Active:       false,
			TokenVersion: 1,
		}, nil)

		resp, err := New(users, validate, tokens).Execute(context.Background(), "refresh-token")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrUnauthorized)
	})

	t.Run("token version mismatch returns ErrInvalidToken", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		validate := NewMockTokenValidator(t)
		tokens := NewMockTokenIssuer(t)

		userID := uuid.New()
		validate.EXPECT().ValidateToken("refresh-token").Return(authcontract.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			Type:         "refresh",
			TokenVersion: 1,
		}, nil)

		users.EXPECT().GetByID(mock.Anything, userID).Return(usercontract.User{
			ID:           userID,
			Email:        "test@example.com",
			Active:       true,
			TokenVersion: 2,
		}, nil)

		resp, err := New(users, validate, tokens).Execute(context.Background(), "refresh-token")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrInvalidToken)
	})

	t.Run("GetByID error propagates", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserProvider(t)
		validate := NewMockTokenValidator(t)
		tokens := NewMockTokenIssuer(t)

		userID := uuid.New()
		validate.EXPECT().ValidateToken("refresh-token").Return(authcontract.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			Type:         "refresh",
			TokenVersion: 1,
		}, nil)

		dbErr := errors.New("database connection lost")
		users.EXPECT().GetByID(mock.Anything, userID).Return(usercontract.User{}, dbErr)

		resp, err := New(users, validate, tokens).Execute(context.Background(), "refresh-token")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})
}
