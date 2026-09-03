package auth

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/auth/domain"
	"github.com/residwi/go-api-project-template/internal/features/user"
)

type UserDirectory interface {
	GetByEmail(ctx context.Context, email string) (user.Credentials, error)
	Create(ctx context.Context, p user.NewUser) (user.Profile, error)
	GetByID(ctx context.Context, id uuid.UUID) (user.Profile, error)
	CheckStatus(ctx context.Context, id uuid.UUID) (user.AccountStatus, error)
}

type Tokens interface {
	Issue(claims domain.Claims, kind domain.Kind, ttl time.Duration) (string, error)
	Verify(token string, want domain.Kind) (domain.Claims, error)
}
