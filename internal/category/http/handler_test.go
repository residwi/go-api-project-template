package http_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/category"
	categoryhttp "github.com/residwi/go-api-project-template/internal/category/http"
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

func TestAdminHandler_CreateCategory(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _ := setupCategoryMux(t)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

		body, _ := json.Marshal(map[string]any{
			"name": "New Category",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			Name string `json:"name"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Name string `json:"name"`
		}{Name: "New Category"}, got)

		// The admin endpoint keeps the fuller adminCategoryResponse shape.
		var fields map[string]any
		require.NoError(t, json.Unmarshal(dataJSON, &fields))
		assert.Contains(t, fields, "active")
		assert.Contains(t, fields, "sort_order")
		assert.Contains(t, fields, "created_at")
		assert.Contains(t, fields, "updated_at")
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _ := setupCategoryMux(t)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		body, _ := json.Marshal(map[string]any{
			"name": "Duplicate",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mux, _, _ := setupCategoryMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing name", func(t *testing.T) {
		mux, _, _ := setupCategoryMux(t)

		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})
}

func TestAdminHandler_UpdateCategory(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _ := setupCategoryMux(t)

		catID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, catID).Return(&category.Category{
			ID:        catID,
			Name:      "Old Name",
			Slug:      "old-name",
			Active:    true,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)

		newName := "Updated Name"
		body, _ := json.Marshal(map[string]any{
			"name": newName,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/categories/"+catID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		// The admin endpoint keeps the fuller adminCategoryResponse shape.
		fields, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.Contains(t, fields, "active")
		assert.Contains(t, fields, "sort_order")
		assert.Contains(t, fields, "created_at")
		assert.Contains(t, fields, "updated_at")
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _ := setupCategoryMux(t)

		body, _ := json.Marshal(map[string]string{"name": "test"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/categories/not-a-uuid", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mux, _, _ := setupCategoryMux(t)

		catID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/categories/"+catID.String(), bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error", func(t *testing.T) {
		mux, _, _ := setupCategoryMux(t)

		catID := uuid.New()
		tooLong := strings.Repeat("a", 256)
		body, _ := json.Marshal(map[string]any{
			"name": tooLong,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/categories/"+catID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _ := setupCategoryMux(t)

		catID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, catID).Return(nil, apperror.ErrNotFound)

		newName := "Updated"
		body, _ := json.Marshal(map[string]any{
			"name": newName,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/categories/"+catID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdminHandler_DeleteCategory(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, counter := setupCategoryMux(t)

		catID := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, catID).Return(0, nil)
		repo.EXPECT().Delete(mock.Anything, catID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/"+catID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _ := setupCategoryMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/not-a-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, counter := setupCategoryMux(t)

		catID := uuid.New()
		counter.EXPECT().CountPublished(mock.Anything, catID).Return(0, nil)
		repo.EXPECT().Delete(mock.Anything, catID).Return(apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/"+catID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
