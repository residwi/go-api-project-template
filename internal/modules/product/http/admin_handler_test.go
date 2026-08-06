package http

import (
	"bytes"
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
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestAdminHandler_CreateProduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)

		body, _ := json.Marshal(map[string]any{
			"name":  "New Product",
			"price": 2999,
			"sku":   "SKU-999",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products", bytes.NewReader(body))
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
		}{Name: "New Product"}, got)

		// The admin endpoint keeps the fuller adminProductResponse shape.
		var fields map[string]any
		require.NoError(t, json.Unmarshal(dataJSON, &fields))
		assert.Contains(t, fields, "sku")
		assert.Contains(t, fields, "status")
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(apperror.ErrConflict)

		body, _ := json.Marshal(map[string]any{
			"name":  "Duplicate",
			"price": 1000,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProductMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing name", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProductMux(t)

		body, _ := json.Marshal(map[string]any{
			"price": 1000,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})
}

func TestAdminHandler_GetProduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, prodID).Return(&product.Product{
			ID:        prodID,
			Name:      "Widget",
			Slug:      "widget",
			Price:     money.New(1999, "USD"),
			Status:    "draft",
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, prodID).Return(nil, nil)

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

		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, prodID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products/"+prodID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProductMux(t)

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

func TestAdminHandler_DeleteProduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		repo.EXPECT().Delete(mock.Anything, prodID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/products/"+prodID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProductMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/products/not-a-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("service error not found", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		repo.EXPECT().Delete(mock.Anything, prodID).Return(apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/products/"+prodID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdminHandler_ListProducts(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		now := time.Now()
		repo.EXPECT().ListAdmin(mock.Anything, mock.Anything).Return([]product.Product{
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

		mux, _ := setupProductMux(t)

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

		mux, repo := setupProductMux(t)

		repo.EXPECT().ListAdmin(mock.Anything, mock.Anything).Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("with valid category_id", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		catID := uuid.New()
		repo.EXPECT().ListAdmin(mock.Anything, mock.MatchedBy(func(p product.AdminListParams) bool {
			return p.CategoryID != nil && *p.CategoryID == catID
		})).Return([]product.Product{}, 0, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products?category_id="+catID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestAdminHandler_UpdateProduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, prodID).Return(&product.Product{
			ID:        prodID,
			Name:      "Old Name",
			Slug:      "old-name",
			Price:     money.New(1000, "USD"),
			Status:    "draft",
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)

		newName := "New Name"
		body, _ := json.Marshal(map[string]any{
			"name": newName,
			"sku":  "SKU-999",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/products/"+prodID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		// The admin endpoint keeps the fuller adminProductResponse shape.
		fields, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.Contains(t, fields, "sku")
		assert.Contains(t, fields, "status")
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProductMux(t)

		body, _ := json.Marshal(map[string]string{"name": "test"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/products/not-a-uuid", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProductMux(t)

		prodID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/products/"+prodID.String(),
			bytes.NewReader([]byte("{bad")),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupProductMux(t)

		prodID := uuid.New()
		badStatus := "invalid_status"
		body, _ := json.Marshal(map[string]any{
			"status": badStatus,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/products/"+prodID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	// The three monetary keys move as a group, because an amount and its currency
	// are one value: completing a partial set silently would re-price a product in
	// a denomination the client never named and answer 200.
	//
	// 400, not 422: the body is well-formed and every field passes its own validate
	// tag. The contradiction is between fields, so it is caught after binding.
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"price without currency", map[string]any{"price": 2000}},
		{"currency without price", map[string]any{"currency": "EUR"}},
		{"compare_at_price without price", map[string]any{"compare_at_price": 2500, "currency": "EUR"}},
	} {
		t.Run("rejects "+tc.name+" with 400", func(t *testing.T) {
			t.Parallel()

			// No repo expectation, so mockery fails the test if the request reaches the
			// service instead of being rejected first.
			mux, _ := setupProductMux(t)

			prodID := uuid.New()
			body, _ := json.Marshal(tc.body)

			w := httptest.NewRecorder()
			r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/products/"+prodID.String(), bytes.NewReader(body))
			r.Header.Set("Content-Type", "application/json")

			mux.ServeHTTP(w, r)

			assert.Equal(t, http.StatusBadRequest, w.Code)

			var resp response.Response
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			assert.False(t, resp.Success)
			assert.Contains(t, resp.Error.Message, "price and currency must be supplied together")
		})
	}

	// The complementary accept case: price and currency together are fine, and
	// the pair reaches the service as one denominated value.
	t.Run("accepts price and currency together", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, prodID).Return(&product.Product{
			ID:        prodID,
			Name:      "Widget",
			Slug:      "widget",
			Price:     money.New(1000, "USD"),
			Status:    "draft",
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().Update(mock.Anything, mock.MatchedBy(func(p *product.Product) bool {
			return p.Price == money.New(2000, "EUR")
		})).Return(nil)

		body, _ := json.Marshal(map[string]any{"price": 2000, "currency": "EUR"})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/products/"+prodID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		require.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		fields, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(2000), fields["price"], 0.0001)
		assert.Equal(t, "EUR", fields["currency"])
	})

	t.Run("service error not found", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, prodID).Return(nil, apperror.ErrNotFound)

		newName := "Updated"
		body, _ := json.Marshal(map[string]any{
			"name": newName,
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/products/"+prodID.String(), bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
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

	got := toAdminProductResponse(&product.Product{
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
