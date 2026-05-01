package order_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	orderMocks "github.com/residwi/go-api-project-template/mocks/order"
)

func setupOrderMux(t *testing.T) (
	*http.ServeMux,
	*orderMocks.MockRepository,
	*orderMocks.MockCartProvider,
	*orderMocks.MockInventoryReserver,
	*orderMocks.MockPaymentInitiator,
	*orderMocks.MockPaymentJobCanceller,
	*orderMocks.MockCouponReserver,
	*orderMocks.MockNotificationEnqueuer,
) {
	repo := orderMocks.NewMockRepository(t)
	cart := orderMocks.NewMockCartProvider(t)
	inventory := orderMocks.NewMockInventoryReserver(t)
	payment := orderMocks.NewMockPaymentInitiator(t)
	paymentCancel := orderMocks.NewMockPaymentJobCanceller(t)
	coupons := orderMocks.NewMockCouponReserver(t)
	notifications := orderMocks.NewMockNotificationEnqueuer(t)

	svc := order.NewService(repo, nil, cart, inventory, payment, paymentCancel, coupons, notifications)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	order.RegisterRoutes(authed, admin, order.RouteDeps{
		Validator: v,
		Service:   svc,
	})

	return mux, repo, cart, inventory, payment, paymentCancel, coupons, notifications
}

func setAuthContext(r *http.Request, userID uuid.UUID) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: userID,
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}

// --- Public Handler: ListOrders ---

func TestPublicHandler_ListOrders(t *testing.T) {
	t.Run("success with cursor pagination", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

		userID := uuid.New()
		now := time.Now()
		orders := []order.Order{
			{
				ID:             uuid.New(),
				UserID:         userID,
				Status:         order.StatusAwaitingPayment,
				SubtotalAmount: 5000,
				TotalAmount:    5000,
				Currency:       "USD",
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		}
		repo.EXPECT().ListByUser(mock.Anything, userID, mock.AnythingOfType("core.CursorPage")).Return(orders, nil)

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
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)
		userID := uuid.New()
		repo.EXPECT().ListByUser(mock.Anything, userID, mock.AnythingOfType("core.CursorPage")).Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders", nil)
		r = setAuthContext(r, userID)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})

	t.Run("has more results triggers cursor", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

		userID := uuid.New()
		now := time.Now()
		orders := make([]order.Order, 21)
		for i := range orders {
			orders[i] = order.Order{
				ID:             uuid.New(),
				UserID:         userID,
				Status:         order.StatusAwaitingPayment,
				SubtotalAmount: 5000,
				TotalAmount:    5000,
				Currency:       "USD",
				CreatedAt:      now.Add(-time.Duration(i) * time.Minute),
				UpdatedAt:      now,
			}
		}
		repo.EXPECT().ListByUser(mock.Anything, userID, mock.AnythingOfType("core.CursorPage")).Return(orders, nil)

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

// --- Public Handler: GetOrder ---

func TestPublicHandler_GetOrder(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&order.Order{
			ID:             orderID,
			UserID:         userID,
			Status:         order.StatusPaid,
			SubtotalAmount: 5000,
			TotalAmount:    5000,
			Currency:       "USD",
			CreatedAt:      now,
			UpdatedAt:      now,
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
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+uuid.NewString(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

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
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String(), nil)
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// --- Public Handler: PlaceOrder ---

func TestPublicHandler_PlaceOrder(t *testing.T) {
	t.Run("missing auth context", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("missing idempotency key", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

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
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		userID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders", strings.NewReader("{invalid"))
		r = setAuthContext(r, userID)
		r.Header.Set("Idempotency-Key", uuid.NewString())

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing payment method", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

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
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

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
}

// --- Public Handler: RetryPayment ---

func TestPublicHandler_RetryPayment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _, _, payment, _, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&order.Order{
			ID:          orderID,
			UserID:      userID,
			Status:      order.StatusAwaitingPayment,
			TotalAmount: 5000,
			Currency:    "USD",
			CreatedAt:   now,
			UpdatedAt:   now,
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
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+uuid.NewString()+"/pay", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

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
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/pay", strings.NewReader("{invalid"))
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing payment method", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/pay", strings.NewReader(`{}`))
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)
	})

	t.Run("service error not found", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		w := httptest.NewRecorder()
		body := `{"payment_method_id":"pm_test_123"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/pay", strings.NewReader(body))
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// --- Public Handler: CancelOrder ---

func TestPublicHandler_CancelOrder(t *testing.T) {
	t.Run("service error handled gracefully", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

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
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+uuid.NewString()+"/cancel", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

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

// --- Admin Handler: List ---

func TestAdminHandler_ListAll(t *testing.T) {
	t.Run("success with pagination", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

		now := time.Now()
		orders := []order.Order{
			{
				ID:             uuid.New(),
				UserID:         uuid.New(),
				Status:         order.StatusPaid,
				SubtotalAmount: 10000,
				TotalAmount:    10000,
				Currency:       "USD",
				CreatedAt:      now,
				UpdatedAt:      now,
			},
		}
		repo.EXPECT().ListAdmin(mock.Anything, mock.AnythingOfType("order.AdminListParams")).Return(orders, 1, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders?page=1&page_size=10", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)
		repo.EXPECT().ListAdmin(mock.Anything, mock.AnythingOfType("order.AdminListParams")).Return(nil, 0, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders", nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

// --- Admin Handler: GetOrder ---

func TestAdminHandler_GetOrder(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&order.Order{
			ID:             orderID,
			UserID:         uuid.New(),
			Status:         order.StatusPaid,
			SubtotalAmount: 5000,
			TotalAmount:    5000,
			Currency:       "USD",
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil)
		repo.EXPECT().ListItemsByOrderID(mock.Anything, orderID).Return([]order.Item{}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders/"+orderID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders/bad-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("service error not found", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)
		orderID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/admin/orders/"+orderID.String(), nil)
		mux.ServeHTTP(w, r)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// --- Admin Handler: UpdateStatus ---

func TestAdminHandler_UpdateStatus(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		now := time.Now()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(&order.Order{
			ID:        orderID,
			UserID:    uuid.New(),
			Status:    order.StatusPaid,
			CreatedAt: now,
			UpdatedAt: now,
		}, nil)
		repo.EXPECT().UpdateStatus(mock.Anything, orderID, order.StatusPaid, order.Status("processing")).Return(nil)

		w := httptest.NewRecorder()
		body := `{"status":"processing"}`
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/"+orderID.String()+"/status", strings.NewReader(body))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNoContent, w.Code)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/bad-uuid/status", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/"+orderID.String()+"/status", strings.NewReader("{invalid"))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing status", func(t *testing.T) {
		mux, _, _, _, _, _, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/"+orderID.String()+"/status", strings.NewReader(`{}`))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error not found", func(t *testing.T) {
		mux, repo, _, _, _, _, _, _ := setupOrderMux(t)

		orderID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		w := httptest.NewRecorder()
		body := `{"status":"processing"}`
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/orders/"+orderID.String()+"/status", strings.NewReader(body))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}
