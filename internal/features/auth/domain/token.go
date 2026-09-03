package domain

import "github.com/google/uuid"

type Kind string

const (
	KindAccess  Kind = "access"
	KindRefresh Kind = "refresh"
)

type Claims struct {
	UserID       uuid.UUID
	Role         string
	TokenVersion int
}
