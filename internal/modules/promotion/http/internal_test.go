package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

func newTestPublicHandler() *handler {
	return &handler{
		service:   &promotion.Service{},
		validator: validator.New(),
	}
}

func newTestAdminHandler() *adminHandler {
	return &adminHandler{
		service:   &promotion.Service{},
		validator: validator.New(),
	}
}

func setAuthContext(r *http.Request) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}

func TestHandler_Apply(t *testing.T) {
	h := newTestPublicHandler()

	t.Run("missing auth", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions/apply", nil)
		w := httptest.NewRecorder()

		h.Apply(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions/apply", strings.NewReader("{bad"))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.Apply(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions/apply", strings.NewReader(`{}`))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.Apply(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		success, ok := resp["success"].(bool)
		require.True(t, ok)
		assert.False(t, success)
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "validation failed", errBody["message"])
	})
}

func TestHandler_AdminCreate(t *testing.T) {
	h := newTestAdminHandler()

	t.Run("invalid JSON", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions", strings.NewReader("{bad"))
		w := httptest.NewRecorder()

		h.Create(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/promotions", strings.NewReader(`{}`))
		w := httptest.NewRecorder()

		h.Create(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "validation failed", errBody["message"])
	})
}

func TestHandler_AdminUpdate(t *testing.T) {
	h := newTestAdminHandler()

	t.Run("invalid UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPut, "/promotions/bad", nil)
		r.SetPathValue("id", "bad")
		w := httptest.NewRecorder()

		h.Update(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid id")
	})

	t.Run("invalid JSON", func(t *testing.T) {
		id := uuid.NewString()
		r := httptest.NewRequest(http.MethodPut, "/promotions/"+id, strings.NewReader("{bad"))
		r.SetPathValue("id", id)
		w := httptest.NewRecorder()

		h.Update(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_AdminDelete(t *testing.T) {
	h := newTestAdminHandler()

	t.Run("invalid UUID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodDelete, "/promotions/bad", nil)
		r.SetPathValue("id", "bad")
		w := httptest.NewRecorder()

		h.Delete(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp map[string]any
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		errBody, ok := resp["error"].(map[string]any)
		require.True(t, ok)
		assert.Contains(t, errBody["message"], "invalid id")
	})
}

// TestApplyResponse_OmitsUsageCountersAndLimits pins the plan's callout: the
// public apply response returns only the computed discount, never the
// promotion's internal usage counters or per-user limits. It goes through
// toApplyResponse -- the same construction path the handler uses -- so the key
// set below is the real wire contract rather than a hand-built struct literal.
//
// What this does and does not guarantee: the ElementsMatch below catches any
// field added to applyResponse without `,omitempty`. It does NOT catch one added
// *with* `,omitempty`, because toApplyResponse takes only a code and a discount
// -- any new field is never assigned, so it is always zero and always omitted.
// The backstop for that case is the compiler: widening the response means
// widening this mapper's signature, which breaks every call site. Said plainly
// because a test comment that overclaims is worse than no comment -- it stops
// the next person looking.
func TestApplyResponse_OmitsUsageCountersAndLimits(t *testing.T) {
	got := toApplyResponse("SAVE10", 424242)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"code", "discount"}, keysOf(fields),
		"the apply response must expose exactly the code and the computed discount")
}

func keysOf(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
