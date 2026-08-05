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
	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_ListOrders(t *testing.T) {
	t.Parallel()

	t.Run("success with cursor pagination", func(t *testing.T) {
		t.Parallel()

		mux, repo, _, _ := setupOrderMux(t)

		userID := uuid.New()
		now := time.Now()
		orders := []order.Order{
			{
				ID:        uuid.New(),
				UserID:    userID,
				Status:    order.StatusAwaitingPayment,
				Subtotal:  money.New(5000, "USD"),
				Total:     money.New(5000, "USD"),
				CreatedAt: now,
				UpdatedAt: now,
			},
		}
		repo.EXPECT().ListByUser(mock.Anything, userID, mock.AnythingOfType("paging.CursorPage")).Return(orders, nil)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, repo, _, _ := setupOrderMux(t)
		userID := uuid.New()
		repo.EXPECT().
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

		mux, repo, _, _ := setupOrderMux(t)

		userID := uuid.New()
		now := time.Now()
		orders := make([]order.Order, 21)
		for i := range orders {
			orders[i] = order.Order{
				ID:        uuid.New(),
				UserID:    userID,
				Status:    order.StatusAwaitingPayment,
				Subtotal:  money.New(5000, "USD"),
				Total:     money.New(5000, "USD"),
				CreatedAt: now.Add(-time.Duration(i) * time.Minute),
				UpdatedAt: now,
			}
		}
		repo.EXPECT().ListByUser(mock.Anything, userID, mock.AnythingOfType("paging.CursorPage")).Return(orders, nil)

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

		mux, repo, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&order.Order{
			ID:        orderID,
			UserID:    userID,
			Status:    order.StatusPaid,
			Subtotal:  money.New(5000, "USD"),
			Total:     money.New(5000, "USD"),
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{}, nil)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, repo, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String(), nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_PlaceOrder(t *testing.T) {
	t.Parallel()

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _, _, _ := setupOrderMux(t)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, repo, _, _ := setupOrderMux(t)

		userID := uuid.New()
		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, mock.AnythingOfType("string")).
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

		mux, repo, cart, _ := setupOrderMux(t)

		userID := uuid.New()
		repo.EXPECT().GetByUserIDAndIdempotencyKey(mock.Anything, userID, mock.AnythingOfType("string")).
			Return(nil, apperror.ErrNotFound)
		cart.EXPECT().LockCart(mock.Anything, userID).Return(nil)
		cart.EXPECT().GetCart(mock.Anything, userID).Return(&order.CartSnapshot{
			ID: uuid.New(),
			Items: []order.CartSnapshotItem{
				{
					ProductID: uuid.New(),
					Quantity:  1,
					Name:      "USD item",
					Price:     money.New(5000, "USD"),
					Status:    "published",
				},
				{
					ProductID: uuid.New(),
					Quantity:  1,
					Name:      "IDR item",
					Price:     money.New(5000, "IDR"),
					Status:    "published",
				},
			},
		}, nil)

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

func TestHandler_RetryPayment(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo, _, payment := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&order.Order{
			ID:        orderID,
			UserID:    userID,
			Status:    order.StatusAwaitingPayment,
			Total:     money.New(5000, "USD"),
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		payment.EXPECT().InitiatePayment(mock.Anything, mock.AnythingOfType("order.InitiatePaymentParams")).
			Return(order.PaymentResult{PaymentID: uuid.New()}, nil)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, _, _, _ := setupOrderMux(t)

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

		mux, repo, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		body := `{"payment_method_id":"pm_test_123"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/pay", strings.NewReader(body))
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_CancelOrder(t *testing.T) {
	t.Parallel()

	t.Run("service error handled gracefully", func(t *testing.T) {
		t.Parallel()

		mux, repo, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&order.Order{
			ID:        orderID,
			UserID:    userID,
			Status:    order.StatusPaymentProcessing,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/cancel", nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusConflict, w.Code)
	})

	t.Run("missing auth context", func(t *testing.T) {
		t.Parallel()

		mux, _, _, _ := setupOrderMux(t)

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

		mux, _, _, _ := setupOrderMux(t)

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
}

func TestAddressResponse_JSONRoundTrip(t *testing.T) {
	t.Parallel()

	got := toAddressResponse(&order.Address{
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

	got := toOrderResponse(&order.Order{
		ID:             orderID,
		UserID:         uuid.New(),
		IdempotencyKey: "idem-key-1",
		RequestHash:    "distinguishable-request-hash",
		Status:         order.StatusPaid,
		Subtotal:       money.New(1000, "USD"),
		Discount:       money.New(0, "USD"),
		Total:          money.New(1000, "USD"),
		StockDeducted:  true,
		StockReversed:  true,
		Items: []order.Item{
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

// Only the mocks these tests drive are returned; the rest are still constructed
// because NewService needs all eight.
func setupOrderMux(t *testing.T) (
	*http.ServeMux,
	*MockRepository,
	*MockCartProvider,
	*MockPaymentInitiator,
) {
	repo := NewMockRepository(t)
	cart := NewMockCartProvider(t)
	inventory := NewMockInventoryReserver(t)
	payment := NewMockPaymentInitiator(t)
	paymentCancel := NewMockPaymentJobCanceller(t)
	coupons := NewMockCouponReserver(t)
	notifications := NewMockNotificationEnqueuer(t)

	svc := order.NewService(
		repo,
		testhelper.FakeTxRunner{},
		cart,
		inventory,
		payment,
		paymentCancel,
		coupons,
		notifications,
		testhelper.DiscardLogger(),
	)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	RegisterRoutes(authed, admin, RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo, cart, payment
}

func setAuthContext(r *http.Request, userID uuid.UUID) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: userID,
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
