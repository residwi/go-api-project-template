// Package contract is auth's published surface. It imports no module and no
// platform package. middleware.Auth imports it so that auth does not have to
// import the transport layer to describe a token.
package contract

import "github.com/google/uuid"

// Claims is a decoded, verified token. Type is "access" or "refresh".
type Claims struct {
	UserID       uuid.UUID
	Email        string
	Role         string
	Type         string
	TokenVersion int
}
