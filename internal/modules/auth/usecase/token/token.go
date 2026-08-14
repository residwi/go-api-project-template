package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/auth/contract"
	"github.com/residwi/go-api-project-template/internal/modules/auth/domain"
	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

type jwtClaims struct {
	jwt.RegisteredClaims

	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string
	TokenVersion int
}

type Service struct {
	secret     string
	issuer     string
	accessTTL  time.Duration
	refreshTTL time.Duration
}

func New(secret, issuer string, accessTTL, refreshTTL time.Duration) *Service {
	return &Service{secret: secret, issuer: issuer, accessTTL: accessTTL, refreshTTL: refreshTTL}
}

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

func (s *Service) ValidateToken(tokenString string) (contract.Claims, error) {
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
		return contract.Claims{}, err
	}

	claims, ok := parsed.Claims.(*jwtClaims)
	if !ok || !parsed.Valid {
		return contract.Claims{}, errors.New("invalid token claims")
	}

	return contract.Claims{
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
