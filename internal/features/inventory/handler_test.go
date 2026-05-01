package inventory_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/features/inventory"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	mocks "github.com/residwi/go-api-project-template/mocks/inventory"
)

func setupInventoryMux(t *testing.T) (*http.ServeMux, *mocks.MockRepository) {
	repo := mocks.NewMockRepository(t)
	svc := inventory.NewService(repo, nil)
	v := validator.New()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	inventory.RegisterRoutes(admin, inventory.RouteDeps{Validator: v, Service: svc})

	return mux, repo
}

func TestAdminHandler_GetStock(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		repo.EXPECT().GetStock(mock.Anything, productID).Return(expected, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/inventory/"+productID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
			Reserved  float64 `json:"reserved"`
			Available float64 `json:"available"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
			Reserved  float64 `json:"reserved"`
			Available float64 `json:"available"`
		}{ProductID: productID.String(), Quantity: 100, Reserved: 10, Available: 90}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _ := setupInventoryMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/inventory/not-a-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid product_id", resp.Error.Message)
	})

	t.Run("not found", func(t *testing.T) {
		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		repo.EXPECT().GetStock(mock.Anything, productID).Return(nil, core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/inventory/"+productID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})
}

func TestAdminHandler_Restock(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 150, Reserved: 5, Available: 145}
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(expected, nil)

		body, _ := json.Marshal(inventory.RestockRequest{Quantity: 50})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/"+productID.String()+"/restock", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
			Reserved  float64 `json:"reserved"`
			Available float64 `json:"available"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
			Reserved  float64 `json:"reserved"`
			Available float64 `json:"available"`
		}{ProductID: productID.String(), Quantity: 150, Reserved: 5, Available: 145}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _ := setupInventoryMux(t)

		body, _ := json.Marshal(inventory.RestockRequest{Quantity: 50})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/not-a-uuid/restock", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid product_id", resp.Error.Message)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mux, _ := setupInventoryMux(t)

		productID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/"+productID.String()+"/restock", bytes.NewReader([]byte("{invalid")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})

	t.Run("validation error quantity=0", func(t *testing.T) {
		mux, _ := setupInventoryMux(t)

		productID := uuid.New()
		body, _ := json.Marshal(inventory.RestockRequest{Quantity: 0})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/"+productID.String()+"/restock", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(nil, fmt.Errorf("%w: product not found", core.ErrNotFound))

		body, _ := json.Marshal(inventory.RestockRequest{Quantity: 50})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/"+productID.String()+"/restock", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})
}

func TestAdminHandler_Adjust(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 200, Reserved: 10, Available: 190}
		repo.EXPECT().AdjustStock(mock.Anything, productID, 200).Return(expected, nil)

		body, _ := json.Marshal(inventory.AdjustRequest{Quantity: 200})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/"+productID.String()+"/adjust", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
			Reserved  float64 `json:"reserved"`
			Available float64 `json:"available"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			ProductID string  `json:"product_id"`
			Quantity  float64 `json:"quantity"`
			Reserved  float64 `json:"reserved"`
			Available float64 `json:"available"`
		}{ProductID: productID.String(), Quantity: 200, Reserved: 10, Available: 190}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _ := setupInventoryMux(t)

		body, _ := json.Marshal(inventory.AdjustRequest{Quantity: 200})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/not-a-uuid/adjust", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid product_id", resp.Error.Message)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mux, _ := setupInventoryMux(t)

		productID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/"+productID.String()+"/adjust", bytes.NewReader([]byte("{invalid")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})

	t.Run("validation error quantity=-1", func(t *testing.T) {
		mux, _ := setupInventoryMux(t)

		productID := uuid.New()
		body, _ := json.Marshal(map[string]int{"quantity": -1})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/"+productID.String()+"/adjust", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		repo.EXPECT().AdjustStock(mock.Anything, productID, 200).Return(nil, fmt.Errorf("%w: cannot set stock below reserved quantity", core.ErrBadRequest))

		body, _ := json.Marshal(inventory.AdjustRequest{Quantity: 200})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/admin/inventory/"+productID.String()+"/adjust", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})
}
