package auth

import "github.com/google/uuid"

// ClaimsView is the parsed content of a bearer token: what
// internal/server/middleware reads out of an access token to
// authenticate a request. Named View, not Claims, because domain.Claims
// already names the richer type BuildTokenPair builds from a user.
type ClaimsView struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string
	TokenVersion int
}
