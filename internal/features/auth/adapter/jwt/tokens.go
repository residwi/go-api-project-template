package jwt

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/auth/domain"
)

type Tokens struct {
	secret []byte
	issuer string
}

func New(secret, issuer string) *Tokens {
	return &Tokens{secret: []byte(secret), issuer: issuer}
}

func (t *Tokens) Issue(claims domain.Claims, kind domain.Kind, ttl time.Duration) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    t.issuer,
			Subject:   claims.UserID.String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID:       claims.UserID,
		Role:         claims.Role,
		Kind:         string(kind),
		TokenVersion: claims.TokenVersion,
	})

	signed, err := token.SignedString(t.secret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}

func (t *Tokens) Verify(token string, want domain.Kind) (domain.Claims, error) {
	parsed, err := jwt.ParseWithClaims(
		token, &jwtClaims{}, func(tok *jwt.Token) (any, error) {
			if _, ok := tok.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", tok.Header["alg"])
			}
			return t.secret, nil
		},
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(t.issuer),
	)
	if err != nil {
		return domain.Claims{}, fmt.Errorf("parsing token: %w", err)
	}

	claims, ok := parsed.Claims.(*jwtClaims)
	if !ok || !parsed.Valid {
		return domain.Claims{}, errors.New("invalid token claims")
	}

	if domain.Kind(claims.Kind) != want {
		return domain.Claims{}, fmt.Errorf("token is a %q, want %q", claims.Kind, want)
	}

	return domain.Claims{
		UserID:       claims.UserID,
		Role:         claims.Role,
		TokenVersion: claims.TokenVersion,
	}, nil
}

type jwtClaims struct {
	jwt.RegisteredClaims

	UserID       uuid.UUID
	Role         string
	Kind         string
	TokenVersion int
}
