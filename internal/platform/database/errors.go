package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

const uniqueViolationCode = "23505"

const foreignKeyViolationCode = "23503"

// IsUniqueViolation checks the SQLSTATE, not the message text, which is locale-
// and version-dependent.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}

// IsForeignKeyViolation checks the SQLSTATE, not the message text, which is
// locale- and version-dependent.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode
}
