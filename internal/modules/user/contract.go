package user

import "github.com/google/uuid"

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

type Profile struct {
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

type AccountStatus struct {
	Active       bool
	TokenVersion int
}
