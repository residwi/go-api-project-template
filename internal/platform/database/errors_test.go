package database_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func TestIsUniqueViolation(t *testing.T) {
	t.Run("true for SQLSTATE 23505", func(t *testing.T) {
		err := &pgconn.PgError{Code: "23505", Message: "duplicate key value violates unique constraint"}
		assert.True(t, database.IsUniqueViolation(err))
	})

	t.Run("true when wrapped", func(t *testing.T) {
		err := fmt.Errorf("creating row: %w", &pgconn.PgError{Code: "23505"})
		assert.True(t, database.IsUniqueViolation(err))
	})

	t.Run("false for other SQLSTATE", func(t *testing.T) {
		err := &pgconn.PgError{Code: "23503"} // foreign_key_violation
		assert.False(t, database.IsUniqueViolation(err))
	})

	t.Run("false for non-pg error", func(t *testing.T) {
		assert.False(t, database.IsUniqueViolation(errors.New("some other error")))
	})

	t.Run("false for nil", func(t *testing.T) {
		assert.False(t, database.IsUniqueViolation(nil))
	})
}
