package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type Claims struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string // "access" or "refresh"
	TokenVersion int
}

type jwtClaims struct {
	jwt.RegisteredClaims

	UserID       uuid.UUID `json:"user_id"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	Type         string    `json:"typ"`
	TokenVersion int       `json:"token_version"`
}

func GenerateTokenPair(secret string, issuer string, accessTTL, refreshTTL time.Duration, claims Claims) (accessToken, refreshToken string, err error) {
	accessToken, err = generateToken(secret, issuer, accessTTL, Claims{
		UserID:       claims.UserID,
		Email:        claims.Email,
		Role:         claims.Role,
		Type:         "access",
		TokenVersion: claims.TokenVersion,
	})
	if err != nil {
		return "", "", fmt.Errorf("generating access token: %w", err)
	}

	refreshToken, err = generateToken(secret, issuer, refreshTTL, Claims{
		UserID:       claims.UserID,
		Email:        claims.Email,
		Role:         claims.Role,
		Type:         "refresh",
		TokenVersion: claims.TokenVersion,
	})
	if err != nil {
		return "", "", fmt.Errorf("generating refresh token: %w", err)
	}

	return accessToken, refreshToken, nil
}

func generateToken(secret, issuer string, ttl time.Duration, claims Claims) (string, error) {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    issuer,
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

	return token.SignedString([]byte(secret))
}

func ValidateToken(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &jwtClaims{}, func(token *jwt.Token) (any, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*jwtClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}

	return &Claims{
		UserID:       claims.UserID,
		Email:        claims.Email,
		Role:         claims.Role,
		Type:         claims.Type,
		TokenVersion: claims.TokenVersion,
	}, nil
}
