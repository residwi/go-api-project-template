package http

import (
	"bytes"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Apply_ServiceError(t *testing.T) {
	t.Parallel()

	t.Run("service returns not found", func(t *testing.T) {
		t.Parallel()

		mux, service := setupApplyMux(t)

		service.EXPECT().Apply(mock.Anything, "NOTEXIST", int64(5000)).Return(int64(0), apperror.ErrNotFound)

		body, _ := json.Marshal(map[string]any{
			"code":     "NOTEXIST",
			"subtotal": 5000,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", bytes.NewReader(body))
		r = setPromoAuthContext(r)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("service returns bad request for inactive promo", func(t *testing.T) {
		t.Parallel()

		mux, service := setupApplyMux(t)

		service.EXPECT().Apply(mock.Anything, "INACTIVE", int64(5000)).
			Return(int64(0), apperror.ErrBadRequest)

		body, _ := json.Marshal(map[string]any{
			"code":     "INACTIVE",
			"subtotal": 5000,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", bytes.NewReader(body))
		r = setPromoAuthContext(r)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_Apply_Success(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupApplyMux(t)

		service.EXPECT().Apply(mock.Anything, "SAVE10", int64(5000)).Return(int64(1000), nil)

		body, _ := json.Marshal(map[string]any{
			"code":     "SAVE10",
			"subtotal": 5000,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", bytes.NewReader(body))
		r = setPromoAuthContext(r)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})
}

func TestHandler_Apply(t *testing.T) {
	t.Parallel()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupApplyMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupApplyMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", strings.NewReader("{bad"))
		r = setPromoAuthContext(r)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupApplyMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", strings.NewReader(`{}`))
		r = setPromoAuthContext(r)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})
}

func TestApplyResponse_OmitsUsageCountersAndLimits(t *testing.T) {
	t.Parallel()

	got := toApplyResponse("SAVE10", 424242)

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t, []string{"code", "discount"}, slices.Collect(maps.Keys(fields)),
		"the apply response must expose exactly the code and the computed discount")
}

func setupApplyMux(t *testing.T) (*http.ServeMux, *MockPromotionApplier) {
	t.Helper()

	service := NewMockPromotionApplier(t)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	authed.HandleFunc("POST /promotions/apply", NewHandler(service, v).Apply)

	return mux, service
}

func setPromoAuthContext(r *http.Request) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
