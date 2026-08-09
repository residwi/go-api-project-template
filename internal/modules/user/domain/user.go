// Package domain holds user's aggregate and its role rules. It is
// module-private: what leaves user leaves through a slice's return type or
// contract/.
package domain

import (
	"time"

	"github.com/google/uuid"
)

// Valid roles. The HTTP boundary validates an incoming role against these
// with a `oneof` tag; updaterole and remove each compare a loaded user's
// role against RoleAdmin before touching the admin count.
const (
	RoleUser  = "user"
	RoleAdmin = "admin"
)

type User struct {
	ID           uuid.UUID
	Email        string
	PasswordHash string
	FirstName    string
	LastName     string
	Phone        string
	Role         string
	Active       bool
	TokenVersion int
	CreatedAt    time.Time
	UpdatedAt    time.Time
	DeletedAt    *time.Time
}
