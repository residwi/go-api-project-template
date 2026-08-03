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
	"github.com/residwi/go-api-project-template/internal/modules/category"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

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

func TestToAdminCategoryResponse_KeepsModerationAndAuditFields(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	description := "Phones, laptops and audio"
	parentID := uuid.New()
	got := toAdminCategoryResponse(&category.Category{
		ID:          uuid.New(),
		Name:        "Electronics",
		Slug:        "electronics",
		Description: &description,
		ParentID:    &parentID,
		SortOrder:   3,
		Active:      true,
		CreatedAt:   now,
		UpdatedAt:   now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{
			"id", "name", "slug", "description", "parent_id",
			"sort_order", "active", "created_at", "updated_at",
		},
		slices.Collect(maps.Keys(fields)),
		"every Category field must be present for admin tooling")
	assert.JSONEq(t, `"Phones, laptops and audio"`, string(fields["description"]),
		"description must carry the category's own value -- this is one of the five field mappings "+
			"duplicated between toCategoryResponse and toAdminCategoryResponse, so it needs pinning on both")
	assert.JSONEq(t, `"`+parentID.String()+`"`, string(fields["parent_id"]),
		"parent_id must carry the category's own value")
}
