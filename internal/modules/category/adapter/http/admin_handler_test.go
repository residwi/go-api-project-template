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

	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestAdminHandler_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		service.EXPECT().
			Create(mock.Anything, "New Category", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&domain.Category{
				Name: "New Category",
			}, nil)

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
		t.Parallel()

		mux, service := setupAdminMux(t)

		service.EXPECT().Create(mock.Anything, "Duplicate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errs.ErrConflict)

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
		t.Parallel()

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/categories", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing name", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

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

func TestAdminHandler_Update(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		catID := uuid.New()
		now := time.Now()
		service.EXPECT().
			Update(mock.Anything, catID, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(&domain.Category{
				ID:        catID,
				Name:      "Updated Name",
				Slug:      "updated-name",
				Active:    true,
				CreatedAt: now,
				UpdatedAt: now,
			}, nil)

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
		t.Parallel()

		mux, _ := setupAdminMux(t)

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
		t.Parallel()

		mux, _ := setupAdminMux(t)

		catID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/categories/"+catID.String(),
			bytes.NewReader([]byte("{bad")),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

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
		t.Parallel()

		mux, service := setupAdminMux(t)

		catID := uuid.New()
		service.EXPECT().
			Update(mock.Anything, catID, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
			Return(nil, errs.ErrNotFound)

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

func TestAdminHandler_Delete(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		catID := uuid.New()
		service.EXPECT().Delete(mock.Anything, catID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/"+catID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

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
		t.Parallel()

		mux, service := setupAdminMux(t)

		catID := uuid.New()
		service.EXPECT().Delete(mock.Anything, catID).Return(errs.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/categories/"+catID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// TestToAdminCategoryResponse_KeepsModerationAndAuditFields pins the fuller
// admin shape: create's and update's pre-flatten handlers each carried an
// identical copy of this test (byte-for-byte, testing the same toXResponse
// mapper), so only one survives the merge.
func TestToAdminCategoryResponse_KeepsModerationAndAuditFields(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	description := "Phones, laptops and audio"
	parentID := uuid.New()
	got := toAdminCategoryResponse(&domain.Category{
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
		"description must carry the category's own value")
	assert.JSONEq(t, `"`+parentID.String()+`"`, string(fields["parent_id"]),
		"parent_id must carry the category's own value")
}

func setupAdminMux(t *testing.T) (*http.ServeMux, *MockCategoryManager) {
	t.Helper()

	service := NewMockCategoryManager(t)
	v := validator.New()

	mux := http.NewServeMux()
	admin := web.NewRouter(mux).Group("/api/v1/admin")
	h := NewAdminHandler(service, v)
	admin.HandleFunc("POST /categories", h.Create)
	admin.HandleFunc("PUT /categories/{id}", h.Update)
	admin.HandleFunc("DELETE /categories/{id}", h.Delete)

	return mux, service
}
