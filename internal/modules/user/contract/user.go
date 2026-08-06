// Package contract is user's published surface: the only user types another
// module may name. It imports no module and no platform package, so importing
// it can never pull user's implementation along.
package contract

import "github.com/google/uuid"

// Credentials carries the password hash, so only auth's login path should ask
// for it. User is the same record without the hash, for everything else.
type Credentials struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
	Role         string
	Active       bool
	TokenVersion int
}

type User struct {
	ID           uuid.UUID
	Email        string
	FirstName    string
	LastName     string
	Role         string
	Active       bool
	TokenVersion int
}

type NewUser struct {
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
}

// AccountStatus is what middleware.Auth checks per request: whether the
// account is still active and whether the token's version is still current.
type AccountStatus struct {
	Active       bool
	TokenVersion int
}
