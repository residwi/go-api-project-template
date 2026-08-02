package http_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category"
	categoryhttp "github.com/residwi/go-api-project-template/internal/modules/category/http"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	catMocks "github.com/residwi/go-api-project-template/mocks/category"
)

func setupCategoryMux(t *testing.T) (*http.ServeMux, *catMocks.MockRepository, *catMocks.MockProductCounter) {
	repo := catMocks.NewMockRepository(t)
	counter := catMocks.NewMockProductCounter(t)
	svc := category.NewService(repo, counter)
	v := validator.New()

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	categoryhttp.RegisterRoutes(api, admin, categoryhttp.RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo, counter
}

func TestPublicHandler_ListCategories(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _ := setupCategoryMux(t)

		now := time.Now()
		repo.EXPECT().List(mock.Anything).Return([]category.Category{
			{
				ID:        uuid.New(),
				Name:      "Electronics",
				Slug:      "electronics",
				Active:    true,
				CreatedAt: now,
				UpdatedAt: now,
			},
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.([]any)
		require.True(t, ok)
		assert.Len(t, data, 1)

		item, ok := data[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Electronics", item["name"])
		assert.Equal(t, "electronics", item["slug"])
		assert.NotContains(t, item, "active")
		assert.NotContains(t, item, "sort_order")
		assert.NotContains(t, item, "parent_id")
		assert.NotContains(t, item, "description")
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _ := setupCategoryMux(t)

		repo.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestPublicHandler_GetBySlug(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _ := setupCategoryMux(t)

		catID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetBySlug(mock.Anything, "electronics").Return(&category.Category{
			ID:        catID,
			Name:      "Electronics",
			Slug:      "electronics",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/categories/electronics", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Name string `json:"name"`
			Slug string `json:"slug"`
		}{Name: "Electronics", Slug: "electronics"}, got)

		// active and sort_order are moderation/merchandising details the public
		// endpoint must not expose, even though the fixture set Active: true above.
		var fields map[string]any
		require.NoError(t, json.Unmarshal(dataJSON, &fields))
		assert.NotContains(t, fields, "active")
		assert.NotContains(t, fields, "sort_order")
	})

	t.Run("not found", func(t *testing.T) {
		mux, repo, _ := setupCategoryMux(t)

		repo.EXPECT().GetBySlug(mock.Anything, "nonexistent").Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/categories/nonexistent", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _ := setupCategoryMux(t)

		repo.EXPECT().GetBySlug(mock.Anything, "fail").Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/categories/fail", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
