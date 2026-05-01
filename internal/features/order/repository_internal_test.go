package order

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_searchString(t *testing.T) {
	t.Run("returns true when substring is found", func(t *testing.T) {
		assert.True(t, searchString("duplicate key value violates unique constraint", "duplicate key"))
	})

	t.Run("returns false when substring is not found", func(t *testing.T) {
		assert.False(t, searchString("some other error", "duplicate key"))
	})

	t.Run("returns true when string equals substring", func(t *testing.T) {
		assert.True(t, searchString("exact", "exact"))
	})

	t.Run("returns false for empty string with non-empty substr", func(t *testing.T) {
		assert.False(t, searchString("", "a"))
	})
}
