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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/promotion"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Apply_ServiceError(t *testing.T) {
	t.Parallel()

	t.Run("service returns not found", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupPromotionMux(t)

		repo.EXPECT().GetByCode(mock.Anything, "NOTEXIST").Return(nil, apperror.ErrNotFound)

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

		mux, repo := setupPromotionMux(t)

		repo.EXPECT().GetByCode(mock.Anything, "INACTIVE").Return(&promotion.Promotion{
			ID:        uuid.New(),
			Code:      "INACTIVE",
			Active:    false,
			StartsAt:  time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(time.Hour),
		}, nil)

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

		mux, repo := setupPromotionMux(t)

		repo.EXPECT().GetByCode(mock.Anything, "SAVE10").Return(&promotion.Promotion{
			ID:             uuid.New(),
			Code:           "SAVE10",
			Type:           promotion.TypeFixedAmount,
			Value:          1000,
			MinOrderAmount: 500,
			Active:         true,
			StartsAt:       time.Now().Add(-time.Hour),
			ExpiresAt:      time.Now().Add(time.Hour),
		}, nil)

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

	h := newTestHandler()

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

		r := httptest.NewRequest(http.MethodPost, "/promotions/apply", strings.NewReader("{bad"))
		r = setAuthContext(r)
		w := httptest.NewRecorder()

		h.Apply(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		t.Parallel()

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

func newTestHandler() *handler {
	return &handler{
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

func setupPromotionMux(t *testing.T) (*http.ServeMux, *MockRepository) {
	repo := NewMockRepository(t)
	svc := promotion.NewService(repo, testhelper.FakeTxRunner{})
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	RegisterRoutes(authed, admin, RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo
}

func setPromoAuthContext(r *http.Request) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: uuid.New(),
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
