package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/auth"
	mocks "github.com/residwi/go-api-project-template/mocks/auth"
)

func TestService_Register(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		userID := uuid.New()
		req := auth.RegisterRequest{
			Email:     "test@example.com",
			Password:  "password123",
			FirstName: "John",
			LastName:  "Doe",
		}

		users.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p auth.CreateUserParams) bool {
			return p.Email == req.Email &&
				p.FirstName == req.FirstName &&
				p.LastName == req.LastName &&
				bcrypt.CompareHashAndPassword([]byte(p.PasswordHash), []byte(req.Password)) == nil
		})).Return(auth.UserResult{
			ID:           userID,
			Email:        req.Email,
			FirstName:    req.FirstName,
			LastName:     req.LastName,
			Role:         "customer",
			Active:       true,
			TokenVersion: 0,
		}, nil)

		resp, err := svc.Register(context.Background(), req)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, int(15*time.Minute/time.Second), resp.ExpiresIn)
		assert.Equal(t, auth.UserBrief{
			ID:    userID,
			Email: req.Email,
			Name:  "John Doe",
			Role:  "customer",
		}, resp.User)
	})

	t.Run("Create error propagates", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		users.EXPECT().Create(mock.Anything, mock.Anything).
			Return(auth.UserResult{}, core.ErrConflict)

		resp, err := svc.Register(context.Background(), auth.RegisterRequest{
			Email:     "dup@example.com",
			Password:  "password123",
			FirstName: "John",
			LastName:  "Doe",
		})

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrConflict)
	})

	t.Run("bcrypt error with password exceeding 72 bytes", func(t *testing.T) {
		svc := auth.NewService(nil, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		longPassword := strings.Repeat("a", 73)
		resp, err := svc.Register(context.Background(), auth.RegisterRequest{
			Email:     "test@example.com",
			Password:  longPassword,
			FirstName: "John",
			LastName:  "Doe",
		})

		assert.Nil(t, resp)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "hashing password")
	})
}

func TestService_Login(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		userID := uuid.New()
		hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)

		users.EXPECT().GetByEmail(mock.Anything, "test@example.com").Return(auth.UserCredentials{
			ID:           userID,
			Email:        "test@example.com",
			PasswordHash: string(hash),
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "customer",
			Active:       true,
			TokenVersion: 1,
		}, nil)

		resp, err := svc.Login(context.Background(), auth.LoginRequest{
			Email:    "test@example.com",
			Password: "password123",
		})

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, userID, resp.User.ID)
	})

	t.Run("inactive user returns ErrUnauthorized", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		hash, _ := bcrypt.GenerateFromPassword([]byte("password123"), bcrypt.MinCost)

		users.EXPECT().GetByEmail(mock.Anything, "inactive@example.com").Return(auth.UserCredentials{
			ID:           uuid.New(),
			Email:        "inactive@example.com",
			PasswordHash: string(hash),
			Active:       false,
		}, nil)

		resp, err := svc.Login(context.Background(), auth.LoginRequest{
			Email:    "inactive@example.com",
			Password: "password123",
		})

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrUnauthorized)
	})

	t.Run("wrong password returns ErrInvalidCredentials", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		hash, _ := bcrypt.GenerateFromPassword([]byte("correct-password"), bcrypt.MinCost)

		users.EXPECT().GetByEmail(mock.Anything, "test@example.com").Return(auth.UserCredentials{
			ID:           uuid.New(),
			Email:        "test@example.com",
			PasswordHash: string(hash),
			Active:       true,
		}, nil)

		resp, err := svc.Login(context.Background(), auth.LoginRequest{
			Email:    "test@example.com",
			Password: "wrong-password",
		})

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrInvalidCredentials)
	})

	t.Run("user not found returns ErrInvalidCredentials", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		users.EXPECT().GetByEmail(mock.Anything, "notfound@example.com").Return(auth.UserCredentials{}, errors.New("not found"))

		resp, err := svc.Login(context.Background(), auth.LoginRequest{
			Email:    "notfound@example.com",
			Password: "password123",
		})

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrInvalidCredentials)
	})
}

func TestService_RefreshToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		userID := uuid.New()
		claims := auth.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			TokenVersion: 1,
		}
		_, refreshToken, err := auth.GenerateTokenPair("test-secret", "test-issuer", 15*time.Minute, 24*time.Hour, claims)
		require.NoError(t, err)

		users.EXPECT().GetByID(mock.Anything, userID).Return(auth.UserResult{
			ID:           userID,
			Email:        "test@example.com",
			FirstName:    "John",
			LastName:     "Doe",
			Role:         "customer",
			Active:       true,
			TokenVersion: 1,
		}, nil)

		resp, err := svc.RefreshToken(context.Background(), refreshToken)

		require.NoError(t, err)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
		assert.Equal(t, userID, resp.User.ID)
	})

	t.Run("invalid token returns ErrInvalidToken", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		resp, err := svc.RefreshToken(context.Background(), "invalid-token")

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrInvalidToken)
	})

	t.Run("access token instead of refresh returns ErrInvalidToken", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		claims := auth.Claims{
			UserID:       uuid.New(),
			Email:        "test@example.com",
			Role:         "customer",
			TokenVersion: 1,
		}
		accessToken, _, err := auth.GenerateTokenPair("test-secret", "test-issuer", 15*time.Minute, 24*time.Hour, claims)
		require.NoError(t, err)

		resp, err := svc.RefreshToken(context.Background(), accessToken)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrInvalidToken)
	})

	t.Run("inactive user returns ErrUnauthorized", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		userID := uuid.New()
		claims := auth.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			TokenVersion: 1,
		}
		_, refreshToken, err := auth.GenerateTokenPair("test-secret", "test-issuer", 15*time.Minute, 24*time.Hour, claims)
		require.NoError(t, err)

		users.EXPECT().GetByID(mock.Anything, userID).Return(auth.UserResult{
			ID:           userID,
			Email:        "test@example.com",
			Active:       false,
			TokenVersion: 1,
		}, nil)

		resp, err := svc.RefreshToken(context.Background(), refreshToken)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrUnauthorized)
	})

	t.Run("token version mismatch returns ErrInvalidToken", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		userID := uuid.New()
		claims := auth.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			TokenVersion: 1,
		}
		_, refreshToken, err := auth.GenerateTokenPair("test-secret", "test-issuer", 15*time.Minute, 24*time.Hour, claims)
		require.NoError(t, err)

		users.EXPECT().GetByID(mock.Anything, userID).Return(auth.UserResult{
			ID:           userID,
			Email:        "test@example.com",
			Active:       true,
			TokenVersion: 2,
		}, nil)

		resp, err := svc.RefreshToken(context.Background(), refreshToken)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, core.ErrInvalidToken)
	})

	t.Run("GetByID error propagates", func(t *testing.T) {
		users := mocks.NewMockUserProvider(t)
		svc := auth.NewService(users, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		userID := uuid.New()
		claims := auth.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			TokenVersion: 1,
		}
		_, refreshToken, err := auth.GenerateTokenPair("test-secret", "test-issuer", 15*time.Minute, 24*time.Hour, claims)
		require.NoError(t, err)

		dbErr := errors.New("database connection lost")
		users.EXPECT().GetByID(mock.Anything, userID).Return(auth.UserResult{}, dbErr)

		resp, err := svc.RefreshToken(context.Background(), refreshToken)

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, dbErr)
	})
}

func TestService_ValidateAccessToken(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := auth.NewService(nil, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		userID := uuid.New()
		claims := auth.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			TokenVersion: 1,
		}
		accessToken, _, err := auth.GenerateTokenPair("test-secret", "test-issuer", 15*time.Minute, 24*time.Hour, claims)
		require.NoError(t, err)

		result, err := svc.ValidateAccessToken(accessToken)

		require.NoError(t, err)
		assert.Equal(t, &auth.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			Type:         "access",
			TokenVersion: 1,
		}, result)
	})

	t.Run("expired token returns error", func(t *testing.T) {
		svc := auth.NewService(nil, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)

		claims := auth.Claims{
			UserID:       uuid.New(),
			Email:        "test@example.com",
			Role:         "customer",
			TokenVersion: 1,
		}
		accessToken, _, err := auth.GenerateTokenPair("test-secret", "test-issuer", -1*time.Second, 24*time.Hour, claims)
		require.NoError(t, err)

		result, err := svc.ValidateAccessToken(accessToken)

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}

func TestTokenValidatorAdapter(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		svc := auth.NewService(nil, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)
		adapter := auth.NewTokenValidatorAdapter(svc)

		userID := uuid.New()
		claims := auth.Claims{
			UserID:       userID,
			Email:        "test@example.com",
			Role:         "customer",
			TokenVersion: 2,
		}
		accessToken, _, err := auth.GenerateTokenPair("test-secret", "test-issuer", 15*time.Minute, 24*time.Hour, claims)
		require.NoError(t, err)

		result, err := adapter.ValidateToken(accessToken)

		require.NoError(t, err)
		assert.Equal(t, userID, result.UserID)
		assert.Equal(t, "test@example.com", result.Email)
		assert.Equal(t, "customer", result.Role)
		assert.Equal(t, "access", result.Type)
		assert.Equal(t, 2, result.TokenVersion)
	})

	t.Run("invalid token error", func(t *testing.T) {
		svc := auth.NewService(nil, "test-secret", "test-issuer", 15*time.Minute, 24*time.Hour)
		adapter := auth.NewTokenValidatorAdapter(svc)

		result, err := adapter.ValidateToken("not-a-valid-token")

		assert.Nil(t, result)
		assert.Error(t, err)
	})
}
