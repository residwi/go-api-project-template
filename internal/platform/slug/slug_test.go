package slug

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMake(t *testing.T) {
	t.Parallel()

	t.Run("basic", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "hello-world", Make("Hello World"))
	})

	t.Run("special chars", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "product-1-2024", Make("Product #1 (2024)"))
	})

	t.Run("multiple spaces", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "hello-world", Make("hello   world"))
	})

	t.Run("leading trailing spaces", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "hello-world", Make(" hello world "))
	})

	t.Run("unicode chars stripped", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "caf-rsum", Make("café résumé"))
	})

	t.Run("already slug", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "hello-world", Make("hello-world"))
	})

	t.Run("empty string", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, Make(""))
	})

	t.Run("only special chars", func(t *testing.T) {
		t.Parallel()

		assert.Empty(t, Make("!@#$%"))
	})

	t.Run("mixed case and numbers", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "go-api-v20", Make("Go API v2.0"))
	})

	t.Run("hyphens preserved", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "my-cool-product", Make("my-cool-product"))
	})

	t.Run("multiple hyphens collapsed", func(t *testing.T) {
		t.Parallel()

		assert.Equal(t, "hello-world", Make("hello---world"))
	})
}
