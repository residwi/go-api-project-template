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
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Adjust(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		cmd := NewMockAdjuster(t)

		productID := uuid.New()
		expected := &domain.Stock{ProductID: productID, Quantity: 200, Reserved: 10, Available: 190}
		cmd.EXPECT().Execute(mock.Anything, productID, 200).Return(expected, nil)

		mux := setupAdjustMux(t, cmd)

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

		mux := setupAdjustMux(t, NewMockAdjuster(t))

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

		mux := setupAdjustMux(t, NewMockAdjuster(t))

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

		mux := setupAdjustMux(t, NewMockAdjuster(t))

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

		cmd := NewMockAdjuster(t)

		productID := uuid.New()
		cmd.EXPECT().
			Execute(mock.Anything, productID, 200).
			Return(nil, fmt.Errorf("%w: cannot set stock below reserved quantity", apperror.ErrBadRequest))

		mux := setupAdjustMux(t, cmd)

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

func setupAdjustMux(t *testing.T, cmd Adjuster) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	New(cmd, validator.New()).RegisterHTTP(admin)

	return mux
}
