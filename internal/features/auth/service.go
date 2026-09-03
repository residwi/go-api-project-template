package auth

import (
	"context"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/features/auth/domain"
	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
)

type Service struct {
	users      UserDirectory
	dummyHash  []byte
	bcryptCost int
	tokens     Tokens
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func New(cfg Config, users UserDirectory, tokens Tokens) *Service {
	s := &Service{
		users:      users,
		tokens:     tokens,
		bcryptCost: cfg.BcryptCost,
		accessTTL:  cfg.AccessTokenTTL,
		refreshTTL: cfg.RefreshTokenTTL,
	}
	s.dummyHash, _ = bcrypt.GenerateFromPassword([]byte(dummyPassword), cfg.BcryptCost)
	return s
}

func (s *Service) Login(ctx context.Context, email, password string) (*TokenPair, error) {
	creds, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		return nil, ErrInvalidCredentials
	}

	if !creds.Active {
		return nil, errs.ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	return s.BuildTokenPair(user.Profile{
		ID:           creds.ID,
		Email:        creds.Email,
		FirstName:    creds.FirstName,
		LastName:     creds.LastName,
		Role:         creds.Role,
		Active:       creds.Active,
		TokenVersion: creds.TokenVersion,
	})
}

func (s *Service) Register(
	ctx context.Context,
	email, password, firstName, lastName string,
) (*TokenPair, error) {
	if len(password) > maxPasswordBytes {
		return nil, fmt.Errorf("%w: password must not exceed %d bytes", errs.ErrBadRequest, maxPasswordBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	user, err := s.users.Create(ctx, user.NewUser{
		Email:        email,
		PasswordHash: string(hash),
		FirstName:    firstName,
		LastName:     lastName,
	})
	if err != nil {
		return nil, err
	}

	return s.BuildTokenPair(user)
}

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*TokenPair, error) {
	claims, err := s.tokens.Verify(refreshToken, domain.RefreshToken)
	if err != nil {
		return nil, ErrInvalidToken
	}

	user, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}

	if !user.Active {
		return nil, errs.ErrUnauthorized
	}

	if user.TokenVersion != claims.TokenVersion {
		return nil, ErrInvalidToken
	}

	return s.BuildTokenPair(user)
}

func (s *Service) BuildTokenPair(user user.Profile) (*TokenPair, error) {
	claims := domain.Claims{
		UserID:       user.ID,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
	}

	accessToken, err := s.tokens.Issue(claims, domain.AccessToken, s.accessTTL)
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	refreshToken, err := s.tokens.Issue(claims, domain.RefreshToken, s.refreshTTL)
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.accessTTL.Seconds()),
		User:         user,
	}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (identity.Identity, error) {
	claims, err := s.tokens.Verify(token, domain.AccessToken)
	if err != nil {
		return identity.Identity{}, ErrInvalidToken
	}

	status, err := s.users.CheckStatus(ctx, claims.UserID)
	if err != nil {
		return identity.Identity{}, fmt.Errorf("checking account status: %w", err)
	}

	if !status.Active {
		return identity.Identity{}, ErrAccountDeactivated
	}

	if status.TokenVersion != claims.TokenVersion {
		return identity.Identity{}, ErrTokenRevoked
	}

	return identity.Identity{UserID: claims.UserID, Role: claims.Role}, nil
}

// dummyPassword is hashed once per cost to give the unknown-email login path
// roughly the same latency as a real bcrypt comparison.
const dummyPassword = "invalid-user-timing-equalizer"

// maxPasswordBytes is bcrypt's hard input limit; inputs longer than this error
// in GenerateFromPassword. validator's max=72 counts runes, so we re-check bytes.
const maxPasswordBytes = 72
