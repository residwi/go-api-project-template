package request

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type payload struct {
	Name string `json:"name" validate:"required"`
}

func TestBind(t *testing.T) {
	t.Parallel()

	t.Run("decodes a valid body", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada"}`))

		got, ok := Bind[payload](w, r)

		require.True(t, ok)
		assert.Equal(t, payload{Name: "ada"}, got)
	})

	t.Run("rejects malformed JSON with 400", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`))

		_, ok := Bind[payload](w, r)

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("rejects an unknown field with 400", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"nope":1}`))

		_, ok := Bind[payload](w, r)

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("rejects a body over 1MiB with 400", func(t *testing.T) {
		t.Parallel()

		bigBody := `{"name":"` + strings.Repeat("a", (1<<20)+1024) + `"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bigBody))

		_, ok := Bind[payload](w, r)

		assert.False(t, ok)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Contains(t, w.Body.String(), "request body too large")
	})

	t.Run("reports validation failures as 422 with details", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":""}`))

		_, ok := Bind[payload](w, r)

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

func TestRequireUser(t *testing.T) {
	t.Parallel()

	t.Run("returns the user when present in context", func(t *testing.T) {
		t.Parallel()

		want := identity.Identity{UserID: uuid.New(), Role: "user"}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(identity.NewContext(r.Context(), want))
		w := httptest.NewRecorder()

		got, ok := RequireUser(w, r)

		require.True(t, ok)
		assert.Equal(t, want, got)
		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("writes 401 when no user in context", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		w := httptest.NewRecorder()

		_, ok := RequireUser(w, r)

		assert.False(t, ok)
		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestValidationDetails(t *testing.T) {
	t.Parallel()

	t.Run("returns nil for a valid struct", func(t *testing.T) {
		t.Parallel()

		got := validationDetails(tagStruct{Name: "John", Email: "john@example.com", Role: "admin"})

		assert.Nil(t, got)
	})

	t.Run("names every missing required field", func(t *testing.T) {
		t.Parallel()

		got := validationDetails(tagStruct{})

		assert.Equal(t, map[string]any{
			"name":  "this field is required",
			"email": "this field is required",
			"role":  "this field is required",
		}, got)
	})

	t.Run("reports email, min, max and oneof failures", func(t *testing.T) {
		t.Parallel()

		got := validationDetails(tagStruct{Name: "J", Email: "not-an-email", Role: "moderator"})

		assert.Equal(t, map[string]any{
			"name":  "must be at least 2 characters",
			"email": "must be a valid email address",
			"role":  "must be one of: admin user",
		}, got)

		got = validationDetails(tagStruct{
			Name:  strings.Repeat("a", 51),
			Email: "john@example.com",
			Role:  "user",
		})

		assert.Equal(t, map[string]any{"name": "must be at most 50 characters"}, got)
	})

	t.Run("reports uuid, url, gte and lte failures", func(t *testing.T) {
		t.Parallel()

		got := validationDetails(boundedStruct{
			ID:      "not-a-uuid",
			Website: "not-a-url",
			Age:     10,
			Score:   150,
		})

		assert.Equal(t, map[string]any{
			"iD":      "must be a valid UUID",
			"website": "must be a valid URL",
			"age":     "must be greater than or equal to 18",
			"score":   "must be less than or equal to 100",
		}, got)
	})

	t.Run("falls back to the tag name for an unmapped tag", func(t *testing.T) {
		t.Parallel()

		got := validationDetails(alphanumStruct{Value: "hello world!"})

		assert.Equal(t, map[string]any{"value": "failed on alphanum validation"}, got)
	})

	t.Run("reports a non-struct input under a single error key", func(t *testing.T) {
		t.Parallel()

		got := validationDetails("not a struct")

		require.Contains(t, got, "error")
	})
}

type tagStruct struct {
	Name  string `validate:"required,min=2,max=50"`
	Email string `validate:"required,email"`
	Role  string `validate:"required,oneof=admin user"`
}

type boundedStruct struct {
	ID      string `validate:"required,uuid"`
	Website string `validate:"required,url"`
	Age     int    `validate:"required,gte=18"`
	Score   int    `validate:"required,lte=100"`
}

type alphanumStruct struct {
	Value string `validate:"required,alphanum"`
}
