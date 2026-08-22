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
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/product"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		now := time.Now()
		sku := "SKU-123"
		service.EXPECT().ListPublished(mock.Anything, mock.Anything).Return([]domain.Product{
			{
				ID:        uuid.New(),
				Name:      "Widget",
				Slug:      "widget",
				Price:     money.New(1999, "USD"),
				SKU:       &sku,
				Status:    "published",
				CreatedAt: now,
				UpdatedAt: now,
			},
		}, "", false, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		items, ok := data["items"].([]any)
		require.True(t, ok)
		assert.Len(t, items, 1)

		item, ok := items[0].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, "Widget", item["name"])
		assert.Equal(t, "widget", item["slug"])
		assert.InDelta(t, float64(1999), item["price"], 0.0001)
		assert.Equal(t, "USD", item["currency"])
		// sku and status are merchandising/inventory details the public endpoint
		// must not expose -- see TestToProductResponse_OmitsReservationAndSoftDeleteState.
		assert.NotContains(t, item, "sku")
		assert.NotContains(t, item, "status")

		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, false, pagination["has_more"])
		assert.NotContains(t, pagination, "next_cursor")
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		service.EXPECT().ListPublished(mock.Anything, mock.Anything).Return(nil, "", false, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("invalid category_id", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products?category_id=bad", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid category_id", resp.Error.Message)
	})

	t.Run("invalid min_price", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products?min_price=abc", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid min_price", resp.Error.Message)
	})

	t.Run("invalid max_price", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products?max_price=abc", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid max_price", resp.Error.Message)
	})

	t.Run("with valid filters", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		catID := uuid.New()
		service.EXPECT().ListPublished(mock.Anything, mock.MatchedBy(func(p product.PublishedListParams) bool {
			return p.CategoryID != nil && *p.CategoryID == catID &&
				p.MinPrice != nil && *p.MinPrice == 100 &&
				p.MaxPrice != nil && *p.MaxPrice == 5000
		})).Return(nil, "", false, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodGet,
			"/api/v1/products?category_id="+catID.String()+"&min_price=100&max_price=5000",
			nil,
		)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestHandler_GetBySlug(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		now := time.Now()
		sku := "SKU-123"
		service.EXPECT().GetBySlug(mock.Anything, "widget").Return(&domain.Product{
			ID:        uuid.New(),
			Name:      "Widget",
			Slug:      "widget",
			Price:     money.New(1999, "USD"),
			SKU:       &sku,
			Status:    "published",
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/widget", nil)

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
		}{Name: "Widget", Slug: "widget"}, got)

		// sku and status are merchandising/inventory details the public endpoint
		// must not expose, even though the fixture set both above.
		var fields map[string]any
		require.NoError(t, json.Unmarshal(dataJSON, &fields))
		assert.NotContains(t, fields, "sku")
		assert.NotContains(t, fields, "status")
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		service.EXPECT().GetBySlug(mock.Anything, "nonexistent").Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/nonexistent", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

func TestHandler_GetBySlug_EmptySlug(t *testing.T) {
	t.Parallel()

	h := &Handler{}

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/products/", nil)

	h.GetBySlug(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var resp response.Response
	require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
	assert.False(t, resp.Success)
	assert.Equal(t, "slug is required", resp.Error.Message)
}

func TestToProductResponse_OmitsReservationAndSoftDeleteState(t *testing.T) {
	t.Parallel()

	deletedAt := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	sku := "SKU-DISTINGUISHABLE-424242"

	got := toProductResponse(&domain.Product{
		ID:        uuid.New(),
		Name:      "Widget",
		Slug:      "widget",
		Price:     money.New(1999, "USD"),
		SKU:       &sku,
		Status:    "published",
		DeletedAt: &deletedAt,
		Availability: inventorycontract.Availability{
			OnHand:    50,
			Available: 424242,
		},
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{"id", "name", "slug", "price", "currency", "stock_quantity", "created_at", "updated_at"},
		slices.Collect(maps.Keys(fields)),
		"description, category_id, compare_at_price, and images are omitempty and absent when nil/empty; "+
			"sku and status must never reach the public endpoint")

	assert.NotContains(t, string(raw), "424242",
		"reserved/available stock is live order velocity per SKU and must never be serialised")
	assert.NotContains(t, string(raw), "2026-01-01",
		"a soft-deleted product must not be distinguishable on the wire from one that 404s")
	assert.NotContains(t, string(raw), sku,
		"sku is a merchandising/inventory detail a shopper has no use for")

	var stock struct {
		StockQuantity int `json:"stock_quantity"`
	}
	require.NoError(t, json.Unmarshal(raw, &stock))
	assert.Equal(t, 50, stock.StockQuantity, "stock_quantity must come from Availability.OnHand")
}

func setupMux(t *testing.T) (*http.ServeMux, *MockProductReader) {
	service := NewMockProductReader(t)

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api/v1")

	h := NewHandler(service)
	api.HandleFunc("GET /products", h.List)
	api.HandleFunc("GET /products/{slug}", h.GetBySlug)

	return mux, service
}
