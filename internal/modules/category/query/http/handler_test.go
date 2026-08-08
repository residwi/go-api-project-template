package http

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/category/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupQueryMux(t)

		now := time.Now()
		reader.EXPECT().List(mock.Anything).Return([]domain.Category{
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

	t.Run("reader error", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupQueryMux(t)

		reader.EXPECT().List(mock.Anything).Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/categories", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_GetBySlug(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupQueryMux(t)

		catID := uuid.New()
		now := time.Now()
		reader.EXPECT().GetBySlug(mock.Anything, "electronics").Return(&domain.Category{
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
		t.Parallel()

		mux, reader := setupQueryMux(t)

		reader.EXPECT().GetBySlug(mock.Anything, "nonexistent").Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/categories/nonexistent", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("reader error", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupQueryMux(t)

		reader.EXPECT().GetBySlug(mock.Anything, "fail").Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/categories/fail", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestHandler_GetBySlug_EmptySlug(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/categories/", nil)

	h.getBySlug(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp response.Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Success)
	assert.Equal(t, "slug is required", resp.Error.Message)
}

func TestToCategoryResponse_OmitsModerationAndAuditFields(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	description := "Phones, laptops and audio"
	parentID := uuid.New()
	got := toCategoryResponse(&domain.Category{
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
	assert.ElementsMatch(
		t,
		[]string{"id", "name", "slug", "description", "parent_id"},
		slices.Collect(maps.Keys(fields)),
		"description and parent_id belong to the public shape and must be mapped; sort_order, active, and "+
			"the audit timestamps must never reach the public endpoint",
	)
	assert.JSONEq(t, `"Phones, laptops and audio"`, string(fields["description"]),
		"description must carry the category's own value, not be dropped or defaulted")
}

func setupQueryMux(t *testing.T) (*http.ServeMux, *MockCategoryReader) {
	t.Helper()

	reader := NewMockCategoryReader(t)

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api/v1")
	New(reader).RegisterHTTP(api)

	return mux, reader
}
