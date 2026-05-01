package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/middleware"
)

type UserCredentials struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
	Role         string
	Active       bool
	TokenVersion int
}

type UserResult struct {
	ID           uuid.UUID
	Email        string
	FirstName    string
	LastName     string
	Role         string
	Active       bool
	TokenVersion int
}

type CreateUserParams struct {
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
}

type UserProvider interface {
	GetByEmail(ctx context.Context, email string) (UserCredentials, error)
	Create(ctx context.Context, params CreateUserParams) (UserResult, error)
	GetByID(ctx context.Context, id uuid.UUID) (UserResult, error)
}

type Service struct {
	users      UserProvider
	jwtSecret  string
	jwtIssuer  string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func NewService(users UserProvider, jwtSecret, jwtIssuer string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{
		users:      users,
		jwtSecret:  jwtSecret,
		jwtIssuer:  jwtIssuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
	}
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (*TokenResponse, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	result, err := s.users.Create(ctx, CreateUserParams{
		Email:        req.Email,
		PasswordHash: string(hash),
		FirstName:    req.FirstName,
		LastName:     req.LastName,
	})
	if err != nil {
		return nil, err
	}

	return s.generateTokenResponse(result)
}

func (s *Service) Login(ctx context.Context, req LoginRequest) (*TokenResponse, error) {
	creds, err := s.users.GetByEmail(ctx, req.Email)
	if err != nil {
		return nil, core.ErrInvalidCredentials
	}

	if !creds.Active {
		return nil, core.ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(req.Password)); err != nil {
		return nil, core.ErrInvalidCredentials
	}

	return s.generateTokenResponse(UserResult{
		ID:           creds.ID,
		Email:        creds.Email,
		FirstName:    creds.FirstName,
		LastName:     creds.LastName,
		Role:         creds.Role,
		Active:       creds.Active,
		TokenVersion: creds.TokenVersion,
	})
}

func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*TokenResponse, error) {
	claims, err := ValidateToken(refreshToken, s.jwtSecret)
	if err != nil {
		return nil, core.ErrInvalidToken
	}

	if claims.Type != "refresh" {
		return nil, core.ErrInvalidToken
	}

	result, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}

	if !result.Active {
		return nil, core.ErrUnauthorized
	}

	if result.TokenVersion != claims.TokenVersion {
		return nil, core.ErrInvalidToken
	}

	return s.generateTokenResponse(result)
}

// ValidateAccessToken validates an access token and returns the claims.
func (s *Service) ValidateAccessToken(tokenString string) (*Claims, error) {
	return ValidateToken(tokenString, s.jwtSecret)
}

// TokenValidatorAdapter adapts auth.Service to middleware.TokenValidator
type TokenValidatorAdapter struct {
	service *Service
}

func NewTokenValidatorAdapter(s *Service) *TokenValidatorAdapter {
	return &TokenValidatorAdapter{service: s}
}

func (a *TokenValidatorAdapter) ValidateToken(tokenString string) (*middleware.TokenClaims, error) {
	claims, err := a.service.ValidateAccessToken(tokenString)
	if err != nil {
		return nil, err
	}
	return &middleware.TokenClaims{
		UserID:       claims.UserID,
		Email:        claims.Email,
		Role:         claims.Role,
		Type:         claims.Type,
		TokenVersion: claims.TokenVersion,
	}, nil
}

func (s *Service) generateTokenResponse(user UserResult) (*TokenResponse, error) {
	claims := Claims{
		UserID:       user.ID,
		Email:        user.Email,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
	}

	accessToken, refreshToken, err := GenerateTokenPair(s.jwtSecret, s.jwtIssuer, s.accessTTL, s.refreshTTL, claims)
	if err != nil {
		return nil, err
	}

	return &TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.accessTTL.Seconds()),
		User: UserBrief{
			ID:    user.ID,
			Email: user.Email,
			Name:  user.FirstName + " " + user.LastName,
			Role:  user.Role,
		},
	}, nil
}
