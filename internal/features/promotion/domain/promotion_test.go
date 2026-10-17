package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidCode(t *testing.T) {
	t.Parallel()

	t.Run("accepts upper-case letters and digits", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsValidCode("WELCOME10"))
	})

	t.Run("accepts dash and underscore", func(t *testing.T) {
		t.Parallel()

		assert.True(t, IsValidCode("MAXED-"))
		assert.True(t, IsValidCode("A_B"))
	})

	t.Run("rejects an empty code", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsValidCode(""))
	})

	t.Run("rejects lower-case letters", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsValidCode("welcome10"))
	})

	t.Run("rejects spaces and punctuation", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsValidCode("SUMMER SALE"))
		assert.False(t, IsValidCode("CODE!"))
	})

	t.Run("rejects a non-ASCII letter", func(t *testing.T) {
		t.Parallel()

		assert.False(t, IsValidCode("CAFÉ"))
	})
}
