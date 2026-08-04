package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	mocks "github.com/residwi/go-api-project-template/mocks/inventory"
)

func TestAdminHandler_GetStock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		repo.EXPECT().GetStock(mock.Anything, productID).Return(nil, apperror.ErrNotFound)

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
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 150, Reserved: 5, Available: 145}
		repo.EXPECT().Restock(mock.Anything, productID, 50).Return(expected, nil)

		body, _ := json.Marshal(map[string]any{"quantity": 50})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/admin/inventory/"+productID.String()+"/restock",
			bytes.NewReader(body),
		)
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
		t.Parallel()

		mux, _ := setupInventoryMux(t)

		body, _ := json.Marshal(map[string]any{"quantity": 50})

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
		t.Parallel()

		mux, _ := setupInventoryMux(t)

		productID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/admin/inventory/"+productID.String()+"/restock",
			bytes.NewReader([]byte("{invalid")),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})

	t.Run("validation error quantity=0", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupInventoryMux(t)

		productID := uuid.New()
		body, _ := json.Marshal(map[string]any{"quantity": 0})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/admin/inventory/"+productID.String()+"/restock",
			bytes.NewReader(body),
		)
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

		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		repo.EXPECT().
			Restock(mock.Anything, productID, 50).
			Return(nil, fmt.Errorf("%w: product not found", apperror.ErrNotFound))

		body, _ := json.Marshal(map[string]any{"quantity": 50})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/admin/inventory/"+productID.String()+"/restock",
			bytes.NewReader(body),
		)
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
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		expected := &inventory.Stock{ProductID: productID, Quantity: 200, Reserved: 10, Available: 190}
		repo.EXPECT().AdjustStock(mock.Anything, productID, 200).Return(expected, nil)

		body, _ := json.Marshal(map[string]any{"quantity": 200})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/admin/inventory/"+productID.String()+"/adjust",
			bytes.NewReader(body),
		)
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
		t.Parallel()

		mux, _ := setupInventoryMux(t)

		body, _ := json.Marshal(map[string]any{"quantity": 200})

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
		t.Parallel()

		mux, _ := setupInventoryMux(t)

		productID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/admin/inventory/"+productID.String()+"/adjust",
			bytes.NewReader([]byte("{invalid")),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})

	t.Run("validation error quantity=-1", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupInventoryMux(t)

		productID := uuid.New()
		body, _ := json.Marshal(map[string]int{"quantity": -1})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/admin/inventory/"+productID.String()+"/adjust",
			bytes.NewReader(body),
		)
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

		mux, repo := setupInventoryMux(t)

		productID := uuid.New()
		repo.EXPECT().
			AdjustStock(mock.Anything, productID, 200).
			Return(nil, fmt.Errorf("%w: cannot set stock below reserved quantity", apperror.ErrBadRequest))

		body, _ := json.Marshal(map[string]any{"quantity": 200})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/admin/inventory/"+productID.String()+"/adjust",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.NotNil(t, resp.Error)
	})
}

// Every inventory route is admin-only, so Reserved is deliberately present
// here -- the reservation-count leak this phase closes is on product's
// public response (see product/http/internal_test.go), not this one.
func TestToStockResponse_ExposesExactFieldSet(t *testing.T) {
	t.Parallel()

	got := toStockResponse(&inventory.Stock{
		ProductID: uuid.New(),
		Quantity:  100,
		Reserved:  30,
		Available: 70,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(
		t,
		[]string{"product_id", "quantity", "reserved", "available"},
		slices.Collect(maps.Keys(fields)),
	)
}

func setupInventoryMux(t *testing.T) (*http.ServeMux, *mocks.MockRepository) {
	repo := mocks.NewMockRepository(t)
	svc := inventory.NewService(repo)
	v := validator.New()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	RegisterRoutes(admin, RouteDeps{Validator: v, Service: svc})

	return mux, repo
}
