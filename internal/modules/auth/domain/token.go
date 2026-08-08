// Package domain holds auth's token types. It is module-private: what leaves
// auth leaves through a slice's return type or contract/.
package domain

import (
	"github.com/google/uuid"

	usercontract "github.com/residwi/go-api-project-template/internal/modules/user/contract"
)

// Claims is what token/ signs into and parses out of a JWT. It is distinct
// from contract.Claims, which is what crosses to middleware and to refresh's
// TokenValidator port: keeping them separate means a field token/ adds for
// its own signing logic does not automatically become part of auth's public
// surface.
type Claims struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string // "access" or "refresh"
	TokenVersion int
}

// TokenPair is a core type, not a wire type: it carries the full
// usercontract.User, so http's mapper decides which fields reach a client.
type TokenPair struct {
	AccessToken  string
	RefreshToken string
	ExpiresIn    int
	User         usercontract.User
}
