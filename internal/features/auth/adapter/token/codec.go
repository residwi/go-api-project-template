package token

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/auth/domain"
)

type Codec struct {
	secret []byte
	issuer string
}

func New(secret, issuer string) *Codec {
	return &Codec{secret: []byte(secret), issuer: issuer}
}

func (c *Codec) Sign(claims domain.Claims, ttl time.Duration) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    c.issuer,
			Subject:   claims.UserID.String(),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(now),
		},
		UserID:       claims.UserID,
		Email:        claims.Email,
		Role:         claims.Role,
		Type:         claims.Type,
		TokenVersion: claims.TokenVersion,
	})

	signed, err := token.SignedString(c.secret)
	if err != nil {
		return "", fmt.Errorf("signing token: %w", err)
	}
	return signed, nil
}

func (c *Codec) Parse(tokenString string) (domain.Claims, error) {
	parsed, err := jwt.ParseWithClaims(
		tokenString, &jwtClaims{}, func(t *jwt.Token) (any, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
			}
			return c.secret, nil
		},
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(c.issuer),
	)
	if err != nil {
		return domain.Claims{}, fmt.Errorf("parsing token: %w", err)
	}

	claims, ok := parsed.Claims.(*jwtClaims)
	if !ok || !parsed.Valid {
		return domain.Claims{}, errors.New("invalid token claims")
	}

	return domain.Claims{
		UserID:       claims.UserID,
		Email:        claims.Email,
		Role:         claims.Role,
		Type:         claims.Type,
		TokenVersion: claims.TokenVersion,
	}, nil
}

type jwtClaims struct {
	jwt.RegisteredClaims

	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string
	TokenVersion int
}
