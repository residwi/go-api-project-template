package user

import "github.com/google/uuid"

type Profile struct {
	ID           uuid.UUID
	Email        string
	FirstName    string
	LastName     string
	Role         string
	Active       bool
	TokenVersion int
}

type Credentials struct {
	Profile

	PasswordHash string
}

type NewUser struct {
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
}
