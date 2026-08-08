package register

import (
	"context"
	"strings"
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

		users := NewMockUserCreator(t)
		tokens := NewMockTokenIssuer(t)

		userID := uuid.New()
		req := Params{
			Email:     "test@example.com",
			Password:  "password123",
			FirstName: "John",
			LastName:  "Doe",
		}
		createdUser := usercontract.User{
			ID:        userID,
			Email:     req.Email,
			FirstName: req.FirstName,
			LastName:  req.LastName,
			Role:      "customer",
			Active:    true,
		}

		users.EXPECT().Create(mock.Anything, mock.MatchedBy(func(p usercontract.NewUser) bool {
			return p.Email == req.Email &&
				p.FirstName == req.FirstName &&
				p.LastName == req.LastName &&
				bcrypt.CompareHashAndPassword([]byte(p.PasswordHash), []byte(req.Password)) == nil
		})).Return(createdUser, nil)
		tokens.EXPECT().BuildTokenPair(createdUser).Return(&domain.TokenPair{
			AccessToken:  "access-token",
			RefreshToken: "refresh-token",
			ExpiresIn:    900,
			User:         createdUser,
		}, nil)

		resp, err := New(users, tokens, bcrypt.MinCost).Execute(context.Background(), req)

		require.NoError(t, err)
		assert.Equal(t, userID, resp.User.ID)
		assert.Equal(t, req.Email, resp.User.Email)
		assert.NotEmpty(t, resp.AccessToken)
		assert.NotEmpty(t, resp.RefreshToken)
	})

	t.Run("Create error propagates", func(t *testing.T) {
		t.Parallel()

		users := NewMockUserCreator(t)
		tokens := NewMockTokenIssuer(t)

		users.EXPECT().Create(mock.Anything, mock.Anything).
			Return(usercontract.User{}, apperror.ErrConflict)

		resp, err := New(users, tokens, bcrypt.MinCost).Execute(context.Background(), Params{
			Email:     "dup@example.com",
			Password:  "password123",
			FirstName: "John",
			LastName:  "Doe",
		})

		assert.Nil(t, resp)
		assert.ErrorIs(t, err, apperror.ErrConflict)
	})

	t.Run("password exceeding 72 bytes is a bad request, not a 500", func(t *testing.T) {
		t.Parallel()

		longPassword := strings.Repeat("a", 73)
		resp, err := New(nil, nil, bcrypt.MinCost).Execute(context.Background(), Params{
			Email:     "test@example.com",
			Password:  longPassword,
			FirstName: "John",
			LastName:  "Doe",
		})

		assert.Nil(t, resp)
		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})
}
