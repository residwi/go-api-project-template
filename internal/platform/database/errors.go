package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolationCode is the PostgreSQL SQLSTATE for a unique-constraint violation.
const uniqueViolationCode = "23505"

// foreignKeyViolationCode is the PostgreSQL SQLSTATE for a foreign-key violation.
const foreignKeyViolationCode = "23503"

// IsUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation, checked via the stable SQLSTATE code rather than the (locale- and
// version-dependent) error message text.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}

// IsForeignKeyViolation reports whether err is a PostgreSQL foreign-key
// violation, checked via the stable SQLSTATE code rather than the (locale- and
// version-dependent) error message text.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == foreignKeyViolationCode
}
