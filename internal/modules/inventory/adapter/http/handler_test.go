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

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/response"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web"
)

func TestHandler_GetStock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		service := NewMockInventoryManager(t)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		service.EXPECT().GetStock(mock.Anything, productID).Return(expected, nil)

		mux := setupMux(t, service)

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

		mux := setupMux(t, NewMockInventoryManager(t))

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

		service := NewMockInventoryManager(t)

		productID := uuid.New()
		service.EXPECT().GetStock(mock.Anything, productID).Return(nil, errs.ErrNotFound)

		mux := setupMux(t, service)

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

func TestHandler_Restock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		service := NewMockInventoryManager(t)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 150, Reserved: 5, Available: 145}
		service.EXPECT().Restock(mock.Anything, productID, 50).Return(expected, nil)

		mux := setupMux(t, service)

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

		mux := setupMux(t, NewMockInventoryManager(t))

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

		mux := setupMux(t, NewMockInventoryManager(t))

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

		mux := setupMux(t, NewMockInventoryManager(t))

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

		service := NewMockInventoryManager(t)

		productID := uuid.New()
		service.EXPECT().
			Restock(mock.Anything, productID, 50).
			Return(nil, fmt.Errorf("%w: product not found", errs.ErrNotFound))

		mux := setupMux(t, service)

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

func TestHandler_Adjust(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		service := NewMockInventoryManager(t)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 200, Reserved: 10, Available: 190}
		service.EXPECT().Adjust(mock.Anything, productID, 200).Return(expected, nil)

		mux := setupMux(t, service)

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

		mux := setupMux(t, NewMockInventoryManager(t))

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

		mux := setupMux(t, NewMockInventoryManager(t))

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

		mux := setupMux(t, NewMockInventoryManager(t))

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

		service := NewMockInventoryManager(t)

		productID := uuid.New()
		service.EXPECT().
			Adjust(mock.Anything, productID, 200).
			Return(nil, fmt.Errorf("%w: cannot set stock below reserved quantity", errs.ErrBadRequest))

		mux := setupMux(t, service)

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

func TestToStockResponse_ExposesExactFieldSet(t *testing.T) {
	t.Parallel()

	got := toStockResponse(&domain.Stock{
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

func setupMux(t *testing.T, service InventoryManager) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	admin := web.NewRouteGroup(mux, "/api/admin")
	h := NewHandler(service, validator.New())
	admin.HandleFunc("GET /inventory/{product_id}", h.GetStock)
	admin.HandleFunc("PUT /inventory/{product_id}/restock", h.Restock)
	admin.HandleFunc("PUT /inventory/{product_id}/adjust", h.Adjust)

	return mux
}
