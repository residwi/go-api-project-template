package request

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/identity"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type payload struct {
	Name string `json:"name" validate:"required"`
}

type nestedPayload struct {
	Address struct {
		ZipCode string `json:"zip_code"`
	} `json:"address"`
}

func TestBind(t *testing.T) {
	t.Parallel()

	t.Run("decodes a valid body and writes nothing", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada"}`))

		got, err := Bind[payload](w, r)

		require.NoError(t, err)
		assert.Equal(t, payload{Name: "ada"}, got)
		assert.Empty(t, w.Body.String())
	})

	t.Run("returns malformed JSON for broken syntax and writes nothing", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{`))

		_, err := Bind[payload](w, r)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		require.EqualError(t, err, "bad request: malformed JSON")
		assert.Empty(t, w.Body.String())
	})

	t.Run("returns malformed JSON for an empty body", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(``))

		_, err := Bind[payload](w, r)

		assert.EqualError(t, err, "bad request: malformed JSON")
	})

	t.Run("returns malformed JSON for a top-level array without naming the Go type", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`[]`))

		_, err := Bind[payload](w, r)

		assert.EqualError(t, err, "bad request: malformed JSON")
	})

	t.Run("returns malformed JSON for a top-level string", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`"x"`))

		_, err := Bind[payload](w, r)

		assert.EqualError(t, err, "bad request: malformed JSON")
	})

	t.Run("returns malformed JSON for a second value after the body", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada"} {"nope":1}`))

		_, err := Bind[payload](w, r)

		assert.EqualError(t, err, "bad request: malformed JSON")
	})

	t.Run("returns malformed JSON for trailing garbage after the body", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"ada"} x`))

		_, err := Bind[payload](w, r)

		assert.EqualError(t, err, "bad request: malformed JSON")
	})

	t.Run("accepts trailing whitespace after the body", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{\"name\":\"ada\"}  \n"))

		got, err := Bind[payload](w, r)

		require.NoError(t, err)
		assert.Equal(t, payload{Name: "ada"}, got)
	})

	t.Run("returns a bad request error naming an unknown field", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"nope":1}`))

		_, err := Bind[payload](w, r)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.EqualError(t, err, `bad request: unknown field "nope"`)
	})

	t.Run("names a wrong-type field by its json name without Go type names", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":1}`))

		_, err := Bind[payload](w, r)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.EqualError(t, err, `bad request: invalid type for "name"`)
	})

	t.Run("names a nested wrong-type field by its json path", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"address":{"zip_code":5}}`))

		_, err := Bind[nestedPayload](w, r)

		assert.EqualError(t, err, `bad request: invalid type for "address.zip_code"`)
	})

	t.Run("returns too large when trailing data crosses the 1MiB cap", func(t *testing.T) {
		t.Parallel()

		body := `{"name":"ada"} "` + strings.Repeat("a", (1<<20)+1024) + `"`
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

		_, err := Bind[payload](w, r)

		assert.EqualError(t, err, "bad request: request body too large")
	})

	t.Run("runs the tags against the zero value for a null body", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`null`))

		_, err := Bind[payload](w, r)

		ve, ok := errors.AsType[*response.ValidationError](err)
		require.True(t, ok)
		assert.Equal(t, &response.ValidationError{Details: map[string]any{"name": "this field is required"}}, ve)
	})

	t.Run("returns a bad request error for a body over 1MiB", func(t *testing.T) {
		t.Parallel()

		bigBody := `{"name":"` + strings.Repeat("a", (1<<20)+1024) + `"}`
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(bigBody))

		_, err := Bind[payload](w, r)

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.EqualError(t, err, "bad request: request body too large")
	})

	t.Run("returns a validation error carrying the field details", func(t *testing.T) {
		t.Parallel()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":""}`))

		_, err := Bind[payload](w, r)

		ve, ok := errors.AsType[*response.ValidationError](err)
		require.True(t, ok)
		assert.Equal(t, &response.ValidationError{Details: map[string]any{"name": "this field is required"}}, ve)
		assert.Empty(t, w.Body.String())
	})
}

func TestParseUUIDParam(t *testing.T) {
	t.Parallel()

	t.Run("returns the parsed id", func(t *testing.T) {
		t.Parallel()

		want := uuid.New()
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.SetPathValue("id", want.String())

		got, err := ParseUUIDParam(r, "id")

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns a bad request error naming the parameter", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.SetPathValue("product_id", "not-a-uuid")

		_, err := ParseUUIDParam(r, "product_id")

		require.ErrorIs(t, err, errs.ErrBadRequest)
		assert.EqualError(t, err, "bad request: invalid product_id")
	})
}

func TestRequireUser(t *testing.T) {
	t.Parallel()

	t.Run("returns the user when present in context", func(t *testing.T) {
		t.Parallel()

		want := identity.Identity{UserID: uuid.New(), Role: "user"}
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r = r.WithContext(identity.NewContext(r.Context(), want))

		got, err := RequireUser(r)

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("returns an unauthorized error when no user is in context", func(t *testing.T) {
		t.Parallel()

		r := httptest.NewRequest(http.MethodGet, "/", nil)

		_, err := RequireUser(r)

		require.ErrorIs(t, err, errs.ErrUnauthorized)
		assert.EqualError(t, err, "unauthorized: authentication required")
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
			"id":      "must be a valid UUID",
			"website": "must be a valid URL",
			"age":     "must be greater than or equal to 18",
			"score":   "must be less than or equal to 100",
		}, got)
	})

	t.Run("keys a field by its json name, not its Go name", func(t *testing.T) {
		t.Parallel()

		got := validationDetails(snakeCaseStruct{MinOrderAmount: -1})

		assert.Equal(t, map[string]any{"min_order_amount": "must be greater than or equal to 0"}, got)
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
	Name  string `json:"name"  validate:"required,min=2,max=50"`
	Email string `json:"email" validate:"required,email"`
	Role  string `json:"role"  validate:"required,oneof=admin user"`
}

type boundedStruct struct {
	ID      string `json:"id"      validate:"required,uuid"`
	Website string `json:"website" validate:"required,url"`
	Age     int    `json:"age"     validate:"required,gte=18"`
	Score   int    `json:"score"   validate:"required,lte=100"`
}

type alphanumStruct struct {
	Value string `json:"value" validate:"required,alphanum"`
}

type snakeCaseStruct struct {
	MinOrderAmount int `json:"min_order_amount,omitempty" validate:"gte=0"`
}
