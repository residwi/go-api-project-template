package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/features/auth/domain"
	"github.com/residwi/go-api-project-template/internal/features/user"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	users      UserDirectory
	dummyHash  []byte
	bcryptCost int
	tokens     Tokens
	accessTTL  time.Duration
	refreshTTL time.Duration
	logger     *slog.Logger
	logKey     []byte
	tracer     trace.Tracer
}

func New(cfg Config, users UserDirectory, tokens Tokens, logger *slog.Logger) *Service {
	s := &Service{
		users:      users,
		tokens:     tokens,
		logger:     logger,
		bcryptCost: cfg.BcryptCost,
		accessTTL:  cfg.AccessTokenTTL,
		refreshTTL: cfg.RefreshTokenTTL,
		tracer:     otel.Tracer("github.com/residwi/go-api-project-template/internal/features/auth"),
	}
	s.dummyHash, _ = bcrypt.GenerateFromPassword([]byte(dummyPassword), cfg.BcryptCost)
	keyMAC := hmac.New(sha256.New, []byte(cfg.Secret))
	keyMAC.Write([]byte("auth.login-log-pseudonym"))
	s.logKey = keyMAC.Sum(nil)
	return s
}

func (s *Service) Login(ctx context.Context, email, password string) (_ *TokenPair, err error) {
	ctx, span := s.tracer.Start(ctx, "auth.Login")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	creds, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		s.logger.WarnContext(ctx, "login failed",
			slog.String("email_pseudonym", s.emailPseudonym(email)), slog.String("reason", "unknown_email"))
		return nil, ErrInvalidCredentials
	}

	// bcrypt must run before the Active check: skipping it for inactive accounts
	// leaked account state via a faster, distinct response (CWE-204 enumeration).
	if bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(password)) != nil {
		s.logger.WarnContext(ctx, "login failed",
			slog.String("user_id", creds.ID.String()), slog.String("reason", "bad_password"))
		return nil, ErrInvalidCredentials
	}

	if !creds.Active {
		s.logger.WarnContext(ctx, "login rejected",
			slog.String("user_id", creds.ID.String()), slog.String("reason", "account_deactivated"))
		return nil, ErrAccountDeactivated
	}

	pair, err := s.BuildTokenPair(creds.Profile)
	if err != nil {
		return nil, err
	}

	return pair, nil
}

func (s *Service) Register(
	ctx context.Context,
	email, password, firstName, lastName string,
) (_ *TokenPair, err error) {
	ctx, span := s.tracer.Start(ctx, "auth.Register")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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

func (s *Service) Refresh(ctx context.Context, refreshToken string) (_ *TokenPair, err error) {
	ctx, span := s.tracer.Start(ctx, "auth.Refresh")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	claims, err := s.tokens.Verify(refreshToken, domain.KindRefresh)
	if err != nil {
		return nil, ErrInvalidToken
	}

	u, err := s.activeProfile(ctx, claims)
	if err != nil {
		return nil, err
	}

	return s.BuildTokenPair(u)
}

func (s *Service) BuildTokenPair(user user.Profile) (*TokenPair, error) {
	claims := domain.Claims{
		UserID:       user.ID,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
	}

	accessToken, err := s.tokens.Issue(claims, domain.KindAccess, s.accessTTL)
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	refreshToken, err := s.tokens.Issue(claims, domain.KindRefresh, s.refreshTTL)
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}

	return &TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    s.accessTTL,
		User:         user,
	}, nil
}

func (s *Service) Authenticate(ctx context.Context, token string) (_ identity.Identity, err error) {
	ctx, span := s.tracer.Start(ctx, "auth.Authenticate")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	claims, err := s.tokens.Verify(token, domain.KindAccess)
	if err != nil {
		return identity.Identity{}, ErrInvalidToken
	}

	profile, err := s.activeProfile(ctx, claims)
	if err != nil {
		return identity.Identity{}, err
	}

	return identity.Identity{UserID: claims.UserID, Role: profile.Role}, nil
}

func (s *Service) activeProfile(ctx context.Context, claims domain.Claims) (user.Profile, error) {
	u, err := s.users.GetProfile(ctx, claims.UserID)
	if err != nil {
		if errors.Is(err, errs.ErrNotFound) {
			return user.Profile{}, ErrInvalidToken
		}
		return user.Profile{}, fmt.Errorf("loading account: %w", err)
	}

	if !u.Active {
		return user.Profile{}, ErrAccountDeactivated
	}

	if u.TokenVersion != claims.TokenVersion {
		return user.Profile{}, ErrTokenRevoked
	}

	return u, nil
}

func (s *Service) emailPseudonym(email string) string {
	mac := hmac.New(sha256.New, s.logKey)
	mac.Write([]byte(strings.ToLower(strings.TrimSpace(email))))
	return hex.EncodeToString(mac.Sum(nil)[:8])
}

// dummyPassword is hashed once per cost to give the unknown-email login path
// roughly the same latency as a real bcrypt comparison.
const dummyPassword = "invalid-user-timing-equalizer"

// maxPasswordBytes is bcrypt's hard input limit; inputs longer than this error
// in GenerateFromPassword. validator's max=72 counts runes, so we re-check bytes.
const maxPasswordBytes = 72
