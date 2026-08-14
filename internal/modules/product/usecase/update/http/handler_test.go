package http

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/product/domain"
	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/update"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_UpdateProduct(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupMux(t)

		prodID := uuid.New()
		sku := "SKU-999"
		cmd.EXPECT().Execute(mock.Anything, prodID, mock.Anything).Return(&domain.Product{
			ID:     prodID,
			Name:   "New Name",
			Slug:   "new-name",
			Price:  money.New(1000, "USD"),
			SKU:    &sku,
			Status: "draft",
		}, nil)

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

		// The admin endpoint keeps the fuller productResponse shape.
		fields, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		assert.Contains(t, fields, "sku")
		assert.Contains(t, fields, "status")
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

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

		mux, _ := setupMux(t)

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

		mux, _ := setupMux(t)

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

			// No cmd expectation, so mockery fails the test if the request reaches the
			// command instead of being rejected first.
			mux, _ := setupMux(t)

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
	// the pair reaches the command as one denominated value.
	t.Run("accepts price and currency together", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupMux(t)

		prodID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, prodID, mock.MatchedBy(func(p update.Params) bool {
			return p.Price != nil && *p.Price == money.New(2000, "EUR")
		})).Return(&domain.Product{
			ID:     prodID,
			Name:   "Widget",
			Slug:   "widget",
			Price:  money.New(2000, "EUR"),
			Status: "draft",
		}, nil)

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

		mux, cmd := setupMux(t)

		prodID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, prodID, mock.Anything).Return(nil, apperror.ErrNotFound)

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

func setupMux(t *testing.T) (*http.ServeMux, *MockProductUpdater) {
	cmd := NewMockProductUpdater(t)
	v := validator.New()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	admin.HandleFunc("PUT /products/{id}", New(cmd, v).Update)

	return mux, cmd
}
