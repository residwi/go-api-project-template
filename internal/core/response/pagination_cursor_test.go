package response_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
)

type pageRow struct {
	ID        uuid.UUID `json:"id"`
	CreatedAt time.Time `json:"created_at"`
}

func pageRowKey(r pageRow) (time.Time, uuid.UUID) { return r.CreatedAt, r.ID }

type cursorPageBody struct {
	Success bool                           `json:"success"`
	Data    core.CursorPageResult[pageRow] `json:"data"`
}

func TestCursorPage(t *testing.T) {
	t.Run("fewer rows than limit: no next cursor, has_more false", func(t *testing.T) {
		w := httptest.NewRecorder()
		rows := []pageRow{
			{ID: uuid.New(), CreatedAt: time.Now()},
			{ID: uuid.New(), CreatedAt: time.Now()},
		}

		response.CursorPage(w, rows, 20, pageRowKey)

		assert.Equal(t, http.StatusOK, w.Code)
		var body cursorPageBody
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.True(t, body.Success)
		assert.Len(t, body.Data.Items, 2)
		assert.False(t, body.Data.Pagination.HasMore)
		assert.Empty(t, body.Data.Pagination.NextCursor)
	})

	t.Run("more rows than limit: slices to limit and encodes the last kept row", func(t *testing.T) {
		w := httptest.NewRecorder()
		first := pageRow{ID: uuid.New(), CreatedAt: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)}
		second := pageRow{ID: uuid.New(), CreatedAt: time.Date(2026, 1, 2, 12, 0, 0, 0, time.UTC)}
		overflow := pageRow{ID: uuid.New(), CreatedAt: time.Date(2026, 1, 3, 12, 0, 0, 0, time.UTC)}

		response.CursorPage(w, []pageRow{first, second, overflow}, 2, pageRowKey)

		var body cursorPageBody
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(t, body.Data.Items, 2)
		assert.True(t, body.Data.Pagination.HasMore)

		// Cursor must encode the last KEPT row (second), in the canonical Z07:00 format.
		want := core.EncodeCursor(second.CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00"), second.ID.String())
		assert.Equal(t, want, body.Data.Pagination.NextCursor)
	})

	t.Run("exactly limit rows: no next cursor", func(t *testing.T) {
		w := httptest.NewRecorder()
		rows := []pageRow{
			{ID: uuid.New(), CreatedAt: time.Now()},
			{ID: uuid.New(), CreatedAt: time.Now()},
		}

		response.CursorPage(w, rows, 2, pageRowKey)

		var body cursorPageBody
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Len(t, body.Data.Items, 2)
		assert.False(t, body.Data.Pagination.HasMore)
		assert.Empty(t, body.Data.Pagination.NextCursor)
	})

	t.Run("empty rows: items is an empty array, has_more false", func(t *testing.T) {
		w := httptest.NewRecorder()

		response.CursorPage(w, []pageRow{}, 20, pageRowKey)

		var body cursorPageBody
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.NotNil(t, body.Data.Items)
		assert.Empty(t, body.Data.Items)
		assert.False(t, body.Data.Pagination.HasMore)
	})
}
