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
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/modules/product/query"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_GetProduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupAdminMux(t)

		prodID := uuid.New()
		now := time.Now()
		reader.EXPECT().GetByID(mock.Anything, prodID).Return(&domain.Product{
			ID:        prodID,
			Name:      "Widget",
			Slug:      "widget",
			Price:     money.New(1999, "USD"),
			Status:    "draft",
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products/"+prodID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

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
		}{Name: "Widget"}, got)
	})

	t.Run("service error not found", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupAdminMux(t)

		prodID := uuid.New()
		reader.EXPECT().GetByID(mock.Anything, prodID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products/"+prodID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products/not-a-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})
}

func TestAdminHandler_ListProducts(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupAdminMux(t)

		now := time.Now()
		reader.EXPECT().ListAdmin(mock.Anything, mock.Anything).Return([]domain.Product{
			{
				ID:        uuid.New(),
				Name:      "Widget",
				Slug:      "widget",
				Price:     money.New(1999, "USD"),
				Status:    "draft",
				CreatedAt: now,
				UpdatedAt: now,
			},
		}, 1, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products", nil)

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
		assert.Equal(t, "draft", item["status"])

		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(1), pagination["current_page"], 0.0001)
		assert.InDelta(t, float64(20), pagination["page_size"], 0.0001)
		assert.InDelta(t, float64(1), pagination["total_items"], 0.0001)
		assert.InDelta(t, float64(1), pagination["total_pages"], 0.0001)
		assert.Equal(t, false, pagination["has_previous"])
		assert.Equal(t, false, pagination["has_next"])
	})

	t.Run("invalid category_id", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?category_id=bad", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid category_id", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupAdminMux(t)

		reader.EXPECT().ListAdmin(mock.Anything, mock.Anything).Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("with valid category_id", func(t *testing.T) {
		t.Parallel()

		mux, reader := setupAdminMux(t)

		catID := uuid.New()
		reader.EXPECT().ListAdmin(mock.Anything, mock.MatchedBy(func(p query.AdminListParams) bool {
			return p.CategoryID != nil && *p.CategoryID == catID
		})).Return([]domain.Product{}, 0, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?category_id="+catID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

// toAdminProductResponse duplicates toProductResponse's twelve field mappings by
// hand, so the fixture sets fields unrelated to SKU and Status too, to catch
// that duplication drifting.
func TestToAdminProductResponse_KeepsSKUAndStatus(t *testing.T) {
	t.Parallel()

	sku := "SKU-123"
	description := "A widget"
	categoryID := uuid.New()
	compareAtPrice := money.New(2999, "USD")

	got := toAdminProductResponse(&domain.Product{
		ID:             uuid.New(),
		Name:           "Widget",
		Slug:           "widget",
		Description:    &description,
		CategoryID:     &categoryID,
		Price:          money.New(1999, "USD"),
		CompareAtPrice: &compareAtPrice,
		SKU:            &sku,
		Status:         "draft",
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
		[]string{
			"id", "name", "slug", "description", "category_id", "price", "compare_at_price",
			"currency", "sku", "status", "stock_quantity", "created_at", "updated_at",
		},
		slices.Collect(maps.Keys(fields)),
		"images is omitempty and absent when empty; every other field, including sku and status, must be "+
			"present for admin tooling")

	assert.JSONEq(t, `"A widget"`, string(fields["description"]),
		"description must carry the product's own value, not be dropped or defaulted")
	assert.JSONEq(t, `2999`, string(fields["compare_at_price"]),
		"compare_at_price must carry the product's own value")

	var admin struct {
		StockQuantity int `json:"stock_quantity"`
	}
	require.NoError(t, json.Unmarshal(raw, &admin))
	assert.Equal(t, 50, admin.StockQuantity,
		"stock_quantity must come from Availability.OnHand -- Available is reservation-adjusted and would "+
			"report a different depth under the same key")
}

func setupAdminMux(t *testing.T) (*http.ServeMux, *MockAdminProductReader) {
	reader := NewMockAdminProductReader(t)

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	ah := NewAdmin(reader)
	admin.HandleFunc("GET /products", ah.List)
	admin.HandleFunc("GET /products/{id}", ah.Get)

	return mux, reader
}
