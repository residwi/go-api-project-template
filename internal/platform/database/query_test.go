package database_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func TestEscapeLike(t *testing.T) {
	t.Run("escapes wildcard, underscore, and backslash", func(t *testing.T) {
		assert.Equal(t, `a\%b\_c\\d`, database.EscapeLike(`a%b_c\d`))
	})

	t.Run("leaves plain text untouched", func(t *testing.T) {
		assert.Equal(t, "plain text", database.EscapeLike("plain text"))
	})

	t.Run("empty string", func(t *testing.T) {
		assert.Empty(t, database.EscapeLike(""))
	})
}

func TestKeysetCursor(t *testing.T) {
	t.Run("appends keyset predicate and args for unqualified columns", func(t *testing.T) {
		cursor := core.EncodeCursor("2024-01-01T00:00:00Z", "11111111-1111-1111-1111-111111111111")

		where, args, argIdx, err := database.KeysetCursor("user_id = $1", []any{"u"}, 2, "created_at, id", cursor)

		require.NoError(t, err)
		assert.Equal(t, "user_id = $1 AND (created_at, id) < ($2, $3)", where)
		assert.Equal(t, []any{"u", "2024-01-01T00:00:00Z", "11111111-1111-1111-1111-111111111111"}, args)
		assert.Equal(t, 4, argIdx)
	})

	t.Run("supports table-qualified keyset columns", func(t *testing.T) {
		cursor := core.EncodeCursor("2024-01-01T00:00:00Z", "22222222-2222-2222-2222-222222222222")

		where, _, argIdx, err := database.KeysetCursor("w.user_id = $1", []any{"u"}, 2, "wi.created_at, wi.id", cursor)

		require.NoError(t, err)
		assert.Equal(t, "w.user_id = $1 AND (wi.created_at, wi.id) < ($2, $3)", where)
		assert.Equal(t, 4, argIdx)
	})

	t.Run("malformed cursor yields ErrBadRequest and leaves inputs unchanged", func(t *testing.T) {
		where, args, argIdx, err := database.KeysetCursor("user_id = $1", []any{"u"}, 2, "created_at, id", "not-base64!!")

		require.ErrorIs(t, err, core.ErrBadRequest)
		assert.Equal(t, "user_id = $1", where)
		assert.Equal(t, []any{"u"}, args)
		assert.Equal(t, 2, argIdx)
	})
}
