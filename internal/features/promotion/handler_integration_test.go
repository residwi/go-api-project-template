package promotion_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/features/promotion"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	promoMocks "github.com/residwi/go-api-project-template/mocks/promotion"
)

func setupPromotionMux(t *testing.T) (*http.ServeMux, *promoMocks.MockRepository) {
	repo := promoMocks.NewMockRepository(t)
	svc := promotion.NewService(repo, nil)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	promotion.RegisterRoutes(authed, admin, promotion.RouteDeps{
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

func TestHandler_Apply_ServiceError(t *testing.T) {
	t.Run("service returns not found", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		repo.EXPECT().GetByCode(mock.Anything, "NOTEXIST").Return(nil, core.ErrNotFound)

		body, _ := json.Marshal(promotion.ApplyRequest{
			Code:     "NOTEXIST",
			Subtotal: 5000,
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
		mux, repo := setupPromotionMux(t)

		repo.EXPECT().GetByCode(mock.Anything, "INACTIVE").Return(&promotion.Promotion{
			ID:        uuid.New(),
			Code:      "INACTIVE",
			Active:    false,
			StartsAt:  time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(time.Hour),
		}, nil)

		body, _ := json.Marshal(promotion.ApplyRequest{
			Code:     "INACTIVE",
			Subtotal: 5000,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/promotions/apply", bytes.NewReader(body))
		r = setPromoAuthContext(r)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_Apply_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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

		body, _ := json.Marshal(promotion.ApplyRequest{
			Code:     "SAVE10",
			Subtotal: 5000,
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

func TestHandler_AdminCreate_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).Return(nil)

		startsAt := time.Now().Truncate(time.Second)
		expiresAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)
		body, _ := json.Marshal(promotion.CreateRequest{
			Code:      "NEW10",
			Type:      promotion.TypePercentage,
			Value:     10,
			StartsAt:  startsAt,
			ExpiresAt: expiresAt,
			Active:    true,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/promotions", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})
}

func TestHandler_AdminCreate_ServiceError(t *testing.T) {
	t.Run("repo conflict", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).Return(core.ErrConflict)

		startsAt := time.Now().Truncate(time.Second)
		expiresAt := time.Now().Add(24 * time.Hour).Truncate(time.Second)
		body, _ := json.Marshal(promotion.CreateRequest{
			Code:      "DUP",
			Type:      promotion.TypePercentage,
			Value:     10,
			StartsAt:  startsAt,
			ExpiresAt: expiresAt,
			Active:    true,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/promotions", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)
	})
}

func TestHandler_AdminList(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		promos := []promotion.Promotion{
			{ID: uuid.New(), Code: "A"},
			{ID: uuid.New(), Code: "B"},
		}
		repo.EXPECT().ListAdmin(mock.Anything, promotion.ListParams{Page: 1, PageSize: 20}).Return(promos, 2, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/promotions", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		repo.EXPECT().ListAdmin(mock.Anything, promotion.ListParams{Page: 1, PageSize: 20}).Return(nil, 0, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/promotions", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_AdminUpdate_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(&promotion.Promotion{
			ID:        id,
			Code:      "OLD",
			Type:      promotion.TypeFixedAmount,
			Value:     500,
			Active:    true,
			StartsAt:  time.Now().Add(-time.Hour),
			ExpiresAt: time.Now().Add(time.Hour),
		}, nil)
		repo.EXPECT().Update(mock.Anything, mock.AnythingOfType("*promotion.Promotion")).Return(nil)

		body, _ := json.Marshal(map[string]string{"code": "UPDATED"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})
}

func TestHandler_AdminUpdate_ServiceError(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		id := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, id).Return(nil, core.ErrNotFound)

		body, _ := json.Marshal(map[string]string{"code": "UPDATED"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_AdminUpdate_ValidationError(t *testing.T) {
	t.Run("invalid type value", func(t *testing.T) {
		mux, _ := setupPromotionMux(t)

		id := uuid.NewString()
		body, _ := json.Marshal(map[string]string{"type": "invalid_type"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id, bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})
}

func TestHandler_AdminUpdate_InvalidJSON(t *testing.T) {
	t.Run("invalid JSON via mux", func(t *testing.T) {
		mux, _ := setupPromotionMux(t)

		id := uuid.NewString()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/"+id, strings.NewReader("{bad"))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})
}

func TestHandler_AdminUpdate_InvalidUUID(t *testing.T) {
	t.Run("invalid UUID via mux", func(t *testing.T) {
		mux, _ := setupPromotionMux(t)

		body, _ := json.Marshal(map[string]string{"code": "test"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/promotions/not-a-uuid", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid id", resp.Error.Message)
	})
}

func TestHandler_AdminDelete_Success(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/promotions/"+id.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func TestHandler_AdminDelete_ServiceError(t *testing.T) {
	t.Run("not found", func(t *testing.T) {
		mux, repo := setupPromotionMux(t)

		id := uuid.New()
		repo.EXPECT().Delete(mock.Anything, id).Return(core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/promotions/"+id.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
