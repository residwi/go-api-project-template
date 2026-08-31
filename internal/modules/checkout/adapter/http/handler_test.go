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
	"github.com/residwi/go-api-project-template/internal/modules/money"
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
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
		usecase.EXPECT().PlaceOrder(mock.Anything, userID, mock.Anything).
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
	// would be a 500. The wrapped errs.ErrBadRequest is what makes the 400, and
	// only a mux-level assertion can see that.
	t.Run("mixed-currency cart is a 400, not a 500", func(t *testing.T) {
		t.Parallel()

		mux, usecase := setupMux(t)

		userID := uuid.New()
		usecase.EXPECT().PlaceOrder(mock.Anything, userID, mock.Anything).
			Return(nil, errors.Join(errs.ErrBadRequest, errors.New("cart contains mixed currencies")))

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

	got := toAddressResponse(&orderdomain.Address{
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

	got := toOrderResponse(&orderdomain.Order{
		ID:             orderID,
		UserID:         uuid.New(),
		IdempotencyKey: "idem-key-1",
		RequestHash:    "distinguishable-request-hash",
		Status:         orderdomain.StatusPaid,
		Subtotal:       money.New(1000, "USD"),
		Discount:       money.New(0, "USD"),
		Total:          money.New(1000, "USD"),
		StockDeducted:  true,
		StockReversed:  true,
		Items: []orderdomain.Item{
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

func TestRetryHandler_RetryPayment(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		service.EXPECT().
			RetryPayment(mock.Anything, userID, orderID, "pm_test_123").
			Return(payment.ChargeResult{PaymentID: uuid.New()}, nil)

		w := httptest.NewRecorder()
		body := `{"payment_method_id":"pm_test_123"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/pay", strings.NewReader(body))
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
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+uuid.NewString()+"/pay", nil)

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
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/bad-uuid/pay", nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("invalid JSON body", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/orders/"+orderID.String()+"/pay",
			strings.NewReader("{invalid"),
		)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing payment method", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/pay", strings.NewReader(`{}`))
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("service error not found", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		service.EXPECT().RetryPayment(mock.Anything, userID, orderID, mock.Anything).
			Return(payment.ChargeResult{}, errs.ErrNotFound)

		w := httptest.NewRecorder()
		body := `{"payment_method_id":"pm_test_123"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/pay", strings.NewReader(body))
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestCancelHandler_CancelOrder(t *testing.T) {
	t.Parallel()

	t.Run("service error handled gracefully", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		service.EXPECT().CancelOrder(mock.Anything, userID, orderID).Return(apperror.ErrOrderCharging)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/cancel", nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+uuid.NewString()+"/cancel", nil)

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
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/not-a-uuid/cancel", nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		service.EXPECT().CancelOrder(mock.Anything, userID, orderID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/cancel", nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})
}

func setupMux(t *testing.T) (*http.ServeMux, *MockCheckout) {
	service := NewMockCheckout(t)
	h := NewHandler(service, validator.New())

	mux := http.NewServeMux()
	authed := web.NewRouter(mux).Group("/api/v1")

	authed.HandleFunc("POST /orders", h.Place)
	authed.HandleFunc("POST /orders/{id}/pay", h.Retry)
	authed.HandleFunc("POST /orders/{id}/cancel", h.Cancel)

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
