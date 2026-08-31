package request

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type payload struct {
	Name string `json:"name"`
}

func TestBind(t *testing.T) {
	t.Parallel()

	t.Run("decodes a valid body", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada"}`))

		got, ok := Bind[payload](w, r, passingValidator{})

		require.True(t, ok)
		assert.Equal(t, payload{Name: "ada"}, got)
	})

	t.Run("treats an empty (non-nil) validation map as success", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada"}`))

		got, ok := Bind[payload](w, r, emptyValidator{})

		require.True(t, ok)
		assert.Equal(t, payload{Name: "ada"}, got)
	})

	t.Run("rejects malformed JSON with 400", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`))

		_, ok := Bind[payload](w, r, passingValidator{})

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("rejects an unknown field with 400", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"nope":1}`))

		_, ok := Bind[payload](w, r, passingValidator{})

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("rejects a body over 1MiB with 400", func(t *testing.T) {
		t.Parallel()

		bigBody := `{"name":"` + strings.Repeat("a", (1<<20)+1024) + `"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bigBody))

		_, ok := Bind[payload](w, r, passingValidator{})

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "request body too large")
	})

	t.Run("reports validation failures as 422 with details", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":""}`))

		_, ok := Bind[payload](w, r, failingValidator{})

		require.False(t, ok)
		require.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var body struct {
			Error struct {
				Details map[string]any `json:"details"`
			} `json:"error"`
		}
		require.NoError(t, json.NewDecoder(w.Body).Decode(&body))
		assert.Equal(t, map[string]any{"name": "this field is required"}, body.Error.Details)
	})
}

func TestParseUUIDParam(t *testing.T) {
	t.Parallel()

	t.Run("returns the parsed id", func(t *testing.T) {
		t.Parallel()

		want := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.SetPathValue("id", want.String())

		got, ok := ParseUUIDParam(w, r, "id")

		require.True(t, ok)
		assert.Equal(t, want, got)
	})

	t.Run("rejects a non-uuid with 400 naming the parameter", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.SetPathValue("product_id", "not-a-uuid")

		_, ok := ParseUUIDParam(w, r, "product_id")

		require.False(t, ok)
		require.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "invalid product_id")
	})
}

type failingValidator struct{}

func (failingValidator) Validate(any) map[string]any {
	return map[string]any{"name": "this field is required"}
}

type passingValidator struct{}

func (passingValidator) Validate(any) map[string]any { return nil }

type emptyValidator struct{}

func (emptyValidator) Validate(any) map[string]any { return map[string]any{} }
