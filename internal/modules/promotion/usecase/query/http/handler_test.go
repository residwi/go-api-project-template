package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/query"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupQueryMux(t)

		promos := []domain.Promotion{
			{ID: uuid.New(), Code: "A"},
			{ID: uuid.New(), Code: "B"},
		}
		reader.EXPECT().
			ListAdmin(mock.Anything, query.Params{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}).
			Return(promos, 2, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/promotions", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("reader error", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupQueryMux(t)

		reader.EXPECT().
			ListAdmin(mock.Anything, query.Params{OffsetPage: paging.OffsetPage{Page: 1, PageSize: 20}}).
			Return(nil, 0, assert.AnError)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/promotions", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func setupQueryMux(t *testing.T) (*http.ServeMux, *MockPromotionLister) {
	t.Helper()

	reader := NewMockPromotionLister(t)

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")
	admin.HandleFunc("GET /promotions", New(reader).List)

	return mux, reader
}
