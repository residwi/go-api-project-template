package database

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// uniqueViolationCode is the PostgreSQL SQLSTATE for a unique-constraint violation.
const uniqueViolationCode = "23505"

// IsUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation, checked via the stable SQLSTATE code rather than the (locale- and
// version-dependent) error message text.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == uniqueViolationCode
}
