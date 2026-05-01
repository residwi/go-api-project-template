package user

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsUniqueViolation(t *testing.T) {
	t.Run("returns true for duplicate key error", func(t *testing.T) {
		err := errors.New("ERROR: duplicate key value violates unique constraint \"users_email_key\" (SQLSTATE 23505)")
		assert.True(t, isUniqueViolation(err))
	})

	t.Run("returns false for non-matching error", func(t *testing.T) {
		err := errors.New("connection refused")
		assert.False(t, isUniqueViolation(err))
	})

	t.Run("returns false for long non-matching error", func(t *testing.T) {
		// String long enough to enter searchString loop but not match
		err := errors.New("ERROR: some other database error that is long enough to search through but does not match")
		assert.False(t, isUniqueViolation(err))
	})

	t.Run("returns false for nil error", func(t *testing.T) {
		assert.False(t, isUniqueViolation(nil))
	})
}
