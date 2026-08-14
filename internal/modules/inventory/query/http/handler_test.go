package http

import (
	"encoding/json"
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
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_GetStock(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		reader := NewMockStockReader(t)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 100, Reserved: 10, Available: 90}
		reader.EXPECT().GetStock(mock.Anything, productID).Return(expected, nil)

		mux := setupQueryMux(t, reader)

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

		mux := setupQueryMux(t, NewMockStockReader(t))

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

		reader := NewMockStockReader(t)

		productID := uuid.New()
		reader.EXPECT().GetStock(mock.Anything, productID).Return(nil, apperror.ErrNotFound)

		mux := setupQueryMux(t, reader)

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

func setupQueryMux(t *testing.T, reader StockReader) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	admin.HandleFunc("GET /inventory/{product_id}", New(reader).GetStock)

	return mux
}
