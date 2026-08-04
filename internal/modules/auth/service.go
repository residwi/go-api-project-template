package auth

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

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

func (s *Service) Register(ctx context.Context, p RegisterParams) (*TokenPair, error) {
	// bcrypt only consumes the first 72 bytes and errors beyond that; validator's
	// max=72 counts runes, so reject overlong multibyte passwords as a 400 here
	// rather than letting bcrypt surface a 500.
	if len(p.Password) > maxPasswordBytes {
		return nil, fmt.Errorf("%w: password must not exceed %d bytes", apperror.ErrBadRequest, maxPasswordBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(p.Password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	result, err := s.users.Create(ctx, CreateUserParams{
		Email:        p.Email,
		PasswordHash: string(hash),
		FirstName:    p.FirstName,
		LastName:     p.LastName,
	})
	if err != nil {
		return nil, err
	}

	return s.buildTokenPair(result)
}

func (s *Service) Login(ctx context.Context, p LoginParams) (*TokenPair, error) {
	creds, err := s.users.GetByEmail(ctx, p.Email)
	if err != nil {
		// Run a dummy comparison so an unknown email takes about as long as a
		// wrong password, removing the timing oracle for account enumeration.
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(p.Password))
		return nil, apperror.ErrInvalidCredentials
	}

	if !creds.Active {
		return nil, apperror.ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(p.Password)); err != nil {
		return nil, apperror.ErrInvalidCredentials
	}

	return s.buildTokenPair(UserResult{
		ID:           creds.ID,
		Email:        creds.Email,
		FirstName:    creds.FirstName,
		LastName:     creds.LastName,
		Role:         creds.Role,
		Active:       creds.Active,
		TokenVersion: creds.TokenVersion,
	})
}

func (s *Service) RefreshToken(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := ValidateToken(refreshToken, s.jwtSecret, s.jwtIssuer)
	if err != nil {
		return nil, apperror.ErrInvalidToken
	}

	if claims.Type != "refresh" {
		return nil, apperror.ErrInvalidToken
	}

	result, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}

	if !result.Active {
		return nil, apperror.ErrUnauthorized
	}

	if result.TokenVersion != claims.TokenVersion {
		return nil, apperror.ErrInvalidToken
	}

	return s.buildTokenPair(result)
}

func (s *Service) ValidateAccessToken(tokenString string) (*Claims, error) {
	return ValidateToken(tokenString, s.jwtSecret, s.jwtIssuer)
}

// TokenValidatorAdapter adapts auth.Service to middleware.TokenValidator.
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

func (s *Service) buildTokenPair(user UserResult) (*TokenPair, error) {
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

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.accessTTL.Seconds()),
		User:         user,
	}, nil
}
