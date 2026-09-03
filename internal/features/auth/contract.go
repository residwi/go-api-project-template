package auth

import "github.com/residwi/go-api-project-template/internal/features/user"

type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	User         user.Profile
}
