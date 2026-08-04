package paging

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecodeCursor(t *testing.T) {
	t.Run("invalid base64", func(t *testing.T) {
		_, _, err := DecodeCursor("not-valid-base64!!!")
		assert.Error(t, err)
	})

	t.Run("invalid format", func(t *testing.T) {
		// Valid base64 but no pipe separator
		_, _, err := DecodeCursor("bm9waXBl") // base64 of "nopipe"
		assert.Error(t, err)
	})

	t.Run("round trip", func(t *testing.T) {
		createdAt := "2024-01-15T10:30:00Z"
		id := "550e8400-e29b-41d4-a716-446655440000"

		encoded := EncodeCursor(createdAt, id)
		assert.NotEmpty(t, encoded)

		decodedCreatedAt, decodedID, err := DecodeCursor(encoded)
		require.NoError(t, err)
		assert.Equal(t, createdAt, decodedCreatedAt)
		assert.Equal(t, id, decodedID)
	})
}

func TestParseCursorPage(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items", nil)
		page := ParseCursorPage(req)

		assert.Equal(t, CursorPage{Cursor: "", Limit: 20}, page)
	})

	t.Run("with values", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?cursor=abc123&limit=50", nil)
		page := ParseCursorPage(req)

		assert.Equal(t, CursorPage{Cursor: "abc123", Limit: 50}, page)
	})

	t.Run("limit too high", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?limit=200", nil)
		page := ParseCursorPage(req)

		assert.Equal(t, CursorPage{Cursor: "", Limit: 20}, page)
	})

	t.Run("limit too low", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?limit=0", nil)
		page := ParseCursorPage(req)

		assert.Equal(t, CursorPage{Cursor: "", Limit: 20}, page)
	})

	t.Run("invalid limit", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/items?limit=abc", nil)
		page := ParseCursorPage(req)

		assert.Equal(t, CursorPage{Cursor: "", Limit: 20}, page)
	})
}

func TestNewCursorPageResult(t *testing.T) {
	t.Run("with items", func(t *testing.T) {
		items := []string{"a", "b", "c"}
		result := NewCursorPageResult(items, "next123", true)

		assert.Equal(t, CursorPageResult[string]{
			Items: []string{"a", "b", "c"},
			Pagination: CursorPagination{
				NextCursor: "next123",
				HasMore:    true,
			},
		}, result)
	})

	t.Run("nil items returns empty slice", func(t *testing.T) {
		result := NewCursorPageResult[string](nil, "", false)

		assert.NotNil(t, result.Items)
		assert.Equal(t, CursorPageResult[string]{
			Items: []string{},
			Pagination: CursorPagination{
				NextCursor: "",
				HasMore:    false,
			},
		}, result)
	})
}
