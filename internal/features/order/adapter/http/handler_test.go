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

	"github.com/residwi/go-api-project-template/internal/features/order/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestHandler_ListOrders(t *testing.T) {
	t.Parallel()

	t.Run("success with cursor pagination", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		now := time.Now()
		orders := []domain.Order{
			{
				ID:        uuid.New(),
				UserID:    userID,
				Status:    domain.StatusAwaitingPayment,
				Subtotal:  money.New(5000, "USD"),
				Total:     money.New(5000, "USD"),
				CreatedAt: now,
				UpdatedAt: now,
			},
		}
		service.EXPECT().ListByUser(mock.Anything, userID, mock.AnythingOfType("paging.CursorPage")).Return(orders, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders?limit=10", nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)
		userID := uuid.New()
		service.EXPECT().
			ListByUser(mock.Anything, userID, mock.AnythingOfType("paging.CursorPage")).
			Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
		r = setAuthContext(r, userID)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("has more results triggers cursor", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		now := time.Now()
		orders := make([]domain.Order, 21)
		for i := range orders {
			orders[i] = domain.Order{
				ID:        uuid.New(),
				UserID:    userID,
				Status:    domain.StatusAwaitingPayment,
				Subtotal:  money.New(5000, "USD"),
				Total:     money.New(5000, "USD"),
				CreatedAt: now.Add(-time.Duration(i) * time.Minute),
				UpdatedAt: now,
			}
		}
		service.EXPECT().ListByUser(mock.Anything, userID, mock.AnythingOfType("paging.CursorPage")).Return(orders, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.Equal(t, true, pagination["has_more"])
		assert.NotEmpty(t, pagination["next_cursor"])
	})
}

func TestHandler_GetOrder(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		now := time.Now()
		service.EXPECT().GetForUser(mock.Anything, userID, orderID).Return(&domain.Order{
			ID:        orderID,
			UserID:    userID,
			Status:    domain.StatusPaid,
			Subtotal:  money.New(5000, "USD"),
			Total:     money.New(5000, "USD"),
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String(), nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+uuid.NewString(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		userID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/not-a-uuid", nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("service error not found", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		service.EXPECT().GetForUser(mock.Anything, userID, orderID).Return(nil, errs.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String(), nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAddressResponse_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	got := toAddressResponse(&domain.Address{
		Street:  "123 Main St",
		City:    "Springfield",
		State:   "IL",
		ZipCode: "62701",
		Country: "US",
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"street":"123 Main St",
		"city":"Springfield",
		"state":"IL",
		"zip_code":"62701",
		"country":"US"
	}`, string(raw))
}

func TestAddressResponse_NilIsNil(t *testing.T) {
	t.Parallel()

	assert.Nil(t, toAddressResponse(nil))
}

func TestToOrderResponse_OmitsSagaAndIdempotencyInternals(t *testing.T) {
	t.Parallel()

	orderID := uuid.New()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

	got := toOrderResponse(&domain.Order{
		ID:             orderID,
		UserID:         uuid.New(),
		IdempotencyKey: "idem-key-1",
		RequestHash:    "distinguishable-request-hash",
		Status:         domain.StatusPaid,
		Subtotal:       money.New(1000, "USD"),
		Discount:       money.New(0, "USD"),
		Total:          money.New(1000, "USD"),
		StockDeducted:  true,
		StockReversed:  true,
		Items: []domain.Item{
			{
				ID:          uuid.New(),
				OrderID:     orderID,
				ProductID:   uuid.New(),
				ProductName: "Widget",
				Price:       money.New(1000, "USD"),
				Quantity:    1,
				Subtotal:    money.New(1000, "USD"),
				CreatedAt:   now,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{
			"id", "user_id", "status", "subtotal_amount", "discount_amount", "total_amount",
			"currency", "items", "created_at", "updated_at",
		},
		slices.Collect(maps.Keys(fields)),
		"idempotency_key, request_hash, stock_deducted, and stock_reversed must not appear")

	assert.JSONEq(t, `1000`, string(fields["total_amount"]))
	assert.JSONEq(t, `"USD"`, string(fields["currency"]))

	assert.NotContains(t, string(raw), "distinguishable-request-hash",
		"RequestHash is an idempotency internal and must not be serialised")
	assert.NotContains(t, string(raw), "idem-key-1",
		"IdempotencyKey must not be serialised")
	assert.NotContains(t, string(raw), "stock_deducted",
		"StockDeducted is saga state and must not be serialised")
	assert.NotContains(t, string(raw), "stock_reversed",
		"StockReversed is saga state and must not be serialised")

	var itemFields []map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(fields["items"], &itemFields))
	require.Len(t, itemFields, 1)
	assert.ElementsMatch(t,
		[]string{"id", "product_id", "product_name", "price", "quantity", "subtotal", "created_at"},
		slices.Collect(maps.Keys(itemFields[0])),
		"order_id must not appear on a line item -- it's an internal join key")
}

// setupMux wires only the authed routes, on a prefix this test picks itself --
// the production paths and groups live in internal/server/routes.
func setupMux(t *testing.T) (*http.ServeMux, *MockOrderReader) {
	service := NewMockOrderReader(t)

	mux := http.NewServeMux()
	authed := web.NewRouter(mux).Group("/api/v1")

	h := NewHandler(service)
	authed.HandleFunc("GET /orders", h.List)
	authed.HandleFunc("GET /orders/{id}", h.Get)

	return mux, service
}

func setAuthContext(r *http.Request, userID uuid.UUID) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: userID,
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
