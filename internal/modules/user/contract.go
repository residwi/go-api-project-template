package user

import "github.com/google/uuid"

// Credentials is what auth reads to check a password and mint a token: the
// full row, password hash included.
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

// Profile is what crosses to auth and middleware: enough to answer "who is
// this" without the credential material Credentials carries. Named Profile,
// not User, because domain.User already names the richer type this module
// works with internally.
type Profile struct {
	ID           uuid.UUID
	Email        string
	FirstName    string
	LastName     string
	Role         string
	Active       bool
	TokenVersion int
}

// NewUser is what auth supplies on registration; Service.Create turns it into
// a stored row and returns the resulting Profile.
type NewUser struct {
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
}

// AccountStatus is the account state a bearer token must still be checked
// against on every request: whether the account is still active, and
// whether the token's version matches the row's (a mismatch means the token
// was revoked after issue).
type AccountStatus struct {
	Active       bool
	TokenVersion int
}
