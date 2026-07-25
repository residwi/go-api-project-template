package slug_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/platform/slug"
)

func TestMake(t *testing.T) {
	t.Run("basic", func(t *testing.T) {
		assert.Equal(t, "hello-world", slug.Make("Hello World"))
	})

	t.Run("special chars", func(t *testing.T) {
		assert.Equal(t, "product-1-2024", slug.Make("Product #1 (2024)"))
	})

	t.Run("multiple spaces", func(t *testing.T) {
		assert.Equal(t, "hello-world", slug.Make("hello   world"))
	})

	t.Run("leading trailing spaces", func(t *testing.T) {
		assert.Equal(t, "hello-world", slug.Make(" hello world "))
	})

	t.Run("unicode chars stripped", func(t *testing.T) {
		assert.Equal(t, "caf-rsum", slug.Make("café résumé"))
	})

	t.Run("already slug", func(t *testing.T) {
		assert.Equal(t, "hello-world", slug.Make("hello-world"))
	})

	t.Run("empty string", func(t *testing.T) {
		assert.Empty(t, slug.Make(""))
	})

	t.Run("only special chars", func(t *testing.T) {
		assert.Empty(t, slug.Make("!@#$%"))
	})

	t.Run("mixed case and numbers", func(t *testing.T) {
		assert.Equal(t, "go-api-v20", slug.Make("Go API v2.0"))
	})

	t.Run("hyphens preserved", func(t *testing.T) {
		assert.Equal(t, "my-cool-product", slug.Make("my-cool-product"))
	})

	t.Run("multiple hyphens collapsed", func(t *testing.T) {
		assert.Equal(t, "hello-world", slug.Make("hello---world"))
	})
}
