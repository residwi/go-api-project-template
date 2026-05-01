package product

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSearchString_NotFound(t *testing.T) {
	t.Run("returns false when substring not found", func(t *testing.T) {
		result := searchString("hello world", "xyz")
		assert.False(t, result)
	})

	t.Run("returns true when substring found", func(t *testing.T) {
		result := searchString("hello world", "world")
		assert.True(t, result)
	})
}
