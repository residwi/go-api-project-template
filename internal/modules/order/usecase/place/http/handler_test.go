package http

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_PlaceOrder(t *testing.T) {
	t.Parallel()

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("missing idempotency key", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		userID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error.Message, "Idempotency-Key")
	})

	t.Run("invalid JSON body", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		userID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader("{invalid"))
		r = setAuthContext(r, userID)
		r.Header.Set("Idempotency-Key", uuid.NewString())

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing payment method", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		userID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(`{}`))
		r = setAuthContext(r, userID)
		r.Header.Set("Idempotency-Key", uuid.NewString())

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error is handled gracefully", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupMux(t)

		userID := uuid.New()
		usecase.EXPECT().Execute(mock.Anything, userID, mock.Anything, mock.AnythingOfType("string")).
			Return(nil, errors.New("database connection error"))

		w := httptest.NewRecorder()
		body := `{"payment_method_id":"pm_test_123"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
		r = setAuthContext(r, userID)
		r.Header.Set("Idempotency-Key", uuid.NewString())

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	// money.ErrCurrencyMismatch is not a case in response.HandleErr, so alone it
	// would be a 500. The wrapped apperror.ErrBadRequest is what makes the 400, and
	// only a mux-level assertion can see that.
	t.Run("mixed-currency cart is a 400, not a 500", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupMux(t)

		userID := uuid.New()
		usecase.EXPECT().Execute(mock.Anything, userID, mock.Anything, mock.AnythingOfType("string")).
			Return(nil, errors.Join(apperror.ErrBadRequest, errors.New("cart contains mixed currencies")))

		w := httptest.NewRecorder()
		body := `{"payment_method_id":"pm_test_123"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader(body))
		r = setAuthContext(r, userID)
		r.Header.Set("Idempotency-Key", uuid.NewString())

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Contains(t, resp.Error.Message, "mixed currencies")
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

	assert.NotContains(t, string(raw), "distinguishable-request-hash",
		"RequestHash is an idempotency internal and must not be serialised")
	assert.NotContains(t, string(raw), "idem-key-1",
		"IdempotencyKey must not be serialised")
	assert.NotContains(t, string(raw), "stock_deducted",
		"StockDeducted is saga state and must not be serialised")
	assert.NotContains(t, string(raw), "stock_reversed",
		"StockReversed is saga state and must not be serialised")
}

func setupMux(t *testing.T) (*http.ServeMux, *MockOrderPlacer) {
	usecase := NewMockOrderPlacer(t)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	authed.HandleFunc("POST /orders", New(usecase, v).Place)

	return mux, usecase
}

func setAuthContext(r *http.Request, userID uuid.UUID) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: userID,
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
