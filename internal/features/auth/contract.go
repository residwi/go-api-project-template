package auth

import (
	"time"

	"github.com/residwi/go-api-project-template/internal/features/user"
)

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    time.Duration
	User         user.Profile
}
