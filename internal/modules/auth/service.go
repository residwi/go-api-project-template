package auth

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

// dummyPassword is hashed once per cost to give the unknown-email login path
// roughly the same latency as a real bcrypt comparison.
const dummyPassword = "invalid-user-timing-equalizer"

// maxPasswordBytes is bcrypt's hard input limit; inputs longer than this error
// in GenerateFromPassword. validator's max=72 counts runes, so we re-check bytes.
const maxPasswordBytes = 72

type jwtClaims struct {
	jwt.RegisteredClaims

	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string
	TokenVersion int
}

type Deps struct {
	Config Config
	Users  UserDirectory
}

type Service struct {
	users      UserDirectory
	dummyHash  []byte
	bcryptCost int
	secret     string
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func New(d Deps) *Service {
	s := &Service{
		users:      d.Users,
		bcryptCost: d.Config.BcryptCost,
		secret:     d.Config.Secret,
		issuer:     d.Config.Issuer,
		accessTTL:  d.Config.AccessTokenTTL,
		refreshTTL: d.Config.RefreshTokenTTL,
	}
	s.dummyHash, _ = bcrypt.GenerateFromPassword([]byte(dummyPassword), d.Config.BcryptCost)
	return s
}

func (s *Service) Login(ctx context.Context, email, password string) (*domain.TokenPair, error) {
	creds, err := s.users.GetByEmail(ctx, email)
	if err != nil {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		return nil, apperror.ErrInvalidCredentials
	}

	if !creds.Active {
		return nil, apperror.ErrUnauthorized
	}

	if err := bcrypt.CompareHashAndPassword([]byte(creds.PasswordHash), []byte(password)); err != nil {
		return nil, apperror.ErrInvalidCredentials
	}

	return s.BuildTokenPair(usercontract.User{
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
) (*domain.TokenPair, error) {
	if len(password) > maxPasswordBytes {
		return nil, fmt.Errorf("%w: password must not exceed %d bytes", apperror.ErrBadRequest, maxPasswordBytes)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), s.bcryptCost)
	if err != nil {
		return nil, fmt.Errorf("hashing password: %w", err)
	}

	user, err := s.users.Create(ctx, usercontract.NewUser{
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

func (s *Service) Refresh(ctx context.Context, refreshToken string) (*domain.TokenPair, error) {
	claims, err := s.ValidateToken(refreshToken)
	if err != nil {
		return nil, apperror.ErrInvalidToken
	}

	if claims.Type != "refresh" {
		return nil, apperror.ErrInvalidToken
	}

	user, err := s.users.GetByID(ctx, claims.UserID)
	if err != nil {
		return nil, err
	}

	if !user.Active {
		return nil, apperror.ErrUnauthorized
	}

	if user.TokenVersion != claims.TokenVersion {
		return nil, apperror.ErrInvalidToken
	}

	return s.BuildTokenPair(user)
}

// BuildTokenPair and ValidateToken keep the names they had as their own
// slice: other code binds to them by name-match (middleware.Auth's
// TokenValidator port, satisfied by *Service directly now that token no
// longer lives behind a separate cross-slice port).
func (s *Service) BuildTokenPair(user usercontract.User) (*domain.TokenPair, error) {
	claims := domain.Claims{
		UserID:       user.ID,
		Email:        user.Email,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
	}

	accessToken, err := s.generateToken(s.accessTTL, claims, "access")
	if err != nil {
		return nil, fmt.Errorf("generating access token: %w", err)
	}

	refreshToken, err := s.generateToken(s.refreshTTL, claims, "refresh")
	if err != nil {
		return nil, fmt.Errorf("generating refresh token: %w", err)
	}

	return &domain.TokenPair{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		ExpiresIn:    int(s.accessTTL.Seconds()),
		User:         user,
	}, nil
}

func (s *Service) ValidateToken(tokenString string) (ClaimsView, error) {
	parsed, err := jwt.ParseWithClaims(
		tokenString, &jwtClaims{}, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return []byte(s.secret), nil
		},
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(s.issuer),
	)
	if err != nil {
		return ClaimsView{}, err
	}

	claims, ok := parsed.Claims.(*jwtClaims)
	if !ok || !parsed.Valid {
		return ClaimsView{}, errors.New("invalid token claims")
	}

	return ClaimsView{
		UserID:       claims.UserID,
		Email:        claims.Email,
		Role:         claims.Role,
		Type:         claims.Type,
		TokenVersion: claims.TokenVersion,
	}, nil
}

func (s *Service) generateToken(ttl time.Duration, claims domain.Claims, kind string) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   claims.UserID.String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID:       claims.UserID,
		Email:        claims.Email,
		Role:         claims.Role,
		Type:         kind,
		TokenVersion: claims.TokenVersion,
	})

	return token.SignedString([]byte(s.secret))
}
