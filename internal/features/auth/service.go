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

// maxPasswordBytes is bcrypt's hard input limit; inputs longer than this error
// in GenerateFromPassword. validator's max=72 counts runes, so we re-check bytes.
const maxPasswordBytes = 72

// dummyPassword is hashed once per cost to give the unknown-email login path
// roughly the same latency as a real bcrypt comparison.
const dummyPassword = "invalid-user-timing-equalizer"

type Service struct {
	users      UserProvider
	jwtSecret  string
	jwtIssuer  string
	accessTTL  time.Duration
	refreshTTL time.Duration
	bcryptCost int
	dummyHash  []byte
}

func NewService(users UserProvider, jwtSecret, jwtIssuer string, accessTTL, refreshTTL time.Duration) *Service {
	s := &Service{
		users:      users,
		jwtSecret:  jwtSecret,
		jwtIssuer:  jwtIssuer,
		accessTTL:  accessTTL,
		refreshTTL: refreshTTL,
		bcryptCost: bcrypt.DefaultCost,
	}
	s.dummyHash, _ = bcrypt.GenerateFromPassword([]byte(dummyPassword), s.bcryptCost)
	return s
}

// SetBcryptCost overrides the password-hashing cost (set once at startup from
// config). Values outside bcrypt's valid range are ignored, keeping the default.
func (s *Service) SetBcryptCost(cost int) {
	if cost < bcrypt.MinCost || cost > bcrypt.MaxCost {
		return
	}
	s.bcryptCost = cost
	s.dummyHash, _ = bcrypt.GenerateFromPassword([]byte(dummyPassword), cost)
}

func (s *Service) Register(ctx context.Context, req RegisterRequest) (*TokenResponse, error) {
	// bcrypt only consumes the first 72 bytes and errors beyond that; validator's
	// max=72 counts runes, so reject overlong multibyte passwords as a 400 here
	// rather than letting bcrypt surface a 500.
	if len(req.Password) > maxPasswordBytes {
		return nil, fmt.Errorf("%w: password must not exceed %d bytes", core.ErrBadRequest, maxPasswordBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), s.bcryptCost)
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
		// Run a dummy comparison so an unknown email takes about as long as a
		// wrong password, removing the timing oracle for account enumeration.
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(req.Password))
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
	claims, err := ValidateToken(refreshToken, s.jwtSecret, s.jwtIssuer)
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
	return ValidateToken(tokenString, s.jwtSecret, s.jwtIssuer)
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
