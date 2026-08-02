package http_test

import (
	"bytes"
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
	"github.com/residwi/go-api-project-template/internal/modules/product"
	producthttp "github.com/residwi/go-api-project-template/internal/modules/product/http"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	prodMocks "github.com/residwi/go-api-project-template/mocks/product"
)

func setupProductMux(t *testing.T) (*http.ServeMux, *prodMocks.MockRepository) {
	repo := prodMocks.NewMockRepository(t)
	inv := prodMocks.NewMockInventoryReader(t)
	inv.EXPECT().GetAvailability(mock.Anything, mock.Anything).
		Return(map[uuid.UUID]product.Availability{}, nil).Maybe()
	reg := prodMocks.NewMockInventoryRegistrar(t)
	reg.EXPECT().EnsureLevel(mock.Anything, mock.Anything).Return(nil).Maybe()
	svc := product.NewService(repo, inv, reg)
	v := validator.New()

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	producthttp.RegisterRoutes(api, admin, producthttp.RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo
}

func TestPublicHandler_ListProducts(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupProductMux(t)

		now := time.Now()
		sku := "SKU-123"
		repo.EXPECT().ListPublished(mock.Anything, mock.Anything).Return([]product.Product{
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
		mux, repo := setupProductMux(t)

		repo.EXPECT().ListPublished(mock.Anything, mock.Anything).Return(nil, "", false, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("invalid category_id", func(t *testing.T) {
		mux, _ := setupProductMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products?category_id=bad", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid category_id", resp.Error.Message)
	})

	t.Run("invalid min_price", func(t *testing.T) {
		mux, _ := setupProductMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products?min_price=abc", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid min_price", resp.Error.Message)
	})

	t.Run("invalid max_price", func(t *testing.T) {
		mux, _ := setupProductMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products?max_price=abc", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid max_price", resp.Error.Message)
	})

	t.Run("with valid filters", func(t *testing.T) {
		mux, repo := setupProductMux(t)

		catID := uuid.New()
		repo.EXPECT().ListPublished(mock.Anything, mock.MatchedBy(func(p product.PublishedListParams) bool {
			return p.CategoryID != nil && *p.CategoryID == catID &&
				p.MinPrice != nil && *p.MinPrice == 100 &&
				p.MaxPrice != nil && *p.MaxPrice == 5000
		})).Return(nil, "", false, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products?category_id="+catID.String()+"&min_price=100&max_price=5000", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func TestPublicHandler_GetBySlug(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		now := time.Now()
		sku := "SKU-123"
		repo.EXPECT().GetBySlug(mock.Anything, "widget").Return(&product.Product{
			ID:        prodID,
			Name:      "Widget",
			Slug:      "widget",
			Price:     money.New(1999, "USD"),
			SKU:       &sku,
			Status:    "published",
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().GetImagesByProductID(mock.Anything, prodID).Return(nil, nil)

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
		mux, repo := setupProductMux(t)

		repo.EXPECT().GetBySlug(mock.Anything, "nonexistent").Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/products/nonexistent", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

func TestAdminHandler_CreateProduct(t *testing.T) {
	t.Run("success", func(t *testing.T) {
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
		mux, _ := setupProductMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/products", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing name", func(t *testing.T) {
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
	t.Run("success", func(t *testing.T) {
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
		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, prodID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products/"+prodID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
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
	t.Run("success", func(t *testing.T) {
		mux, repo := setupProductMux(t)

		prodID := uuid.New()
		repo.EXPECT().Delete(mock.Anything, prodID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodDelete, "/api/v1/admin/products/"+prodID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
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
	t.Run("success", func(t *testing.T) {
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
		mux, repo := setupProductMux(t)

		repo.EXPECT().ListAdmin(mock.Anything, mock.Anything).Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/products", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("with valid category_id", func(t *testing.T) {
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
	t.Run("success", func(t *testing.T) {
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
		mux, _ := setupProductMux(t)

		prodID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/products/"+prodID.String(), bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error", func(t *testing.T) {
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

	// An amount and its currency are one value in product.UpdateParams, so the
	// three monetary keys move as a group. Each case below was accepted before
	// Money and now is not: silently completing them means a client re-prices a
	// product in a denomination it never named (or re-labels one without
	// restating the amount) and gets a 200 back.
	//
	// 400, not 422: the body is well-formed and every field passes its own
	// validate tag -- `omitempty` cannot express "these three travel together",
	// and response.Bind's validation failures are 422. The contradiction is
	// between the fields, which is why it is caught after binding.
	for _, tc := range []struct {
		name string
		body map[string]any
	}{
		{"price without currency", map[string]any{"price": 2000}},
		{"currency without price", map[string]any{"currency": "EUR"}},
		{"compare_at_price without price", map[string]any{"compare_at_price": 2500, "currency": "EUR"}},
	} {
		t.Run("rejects "+tc.name+" with 400", func(t *testing.T) {
			// No repo expectation: the request must be rejected before the service is
			// reached, so the product is never even loaded. mockery fails the test if
			// GetByID or Update is called anyway.
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
