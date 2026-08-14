package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/retrypayment"
	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_RetryPayment(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, cmd := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		cmd.EXPECT().
			Execute(mock.Anything, userID, orderID, retrypayment.Params{PaymentMethodID: "pm_test_123"}).
			Return(&paymentcontract.ChargeResult{PaymentID: uuid.New()}, nil)

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

		mux, cmd := setupMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		cmd.EXPECT().Execute(mock.Anything, userID, orderID, mock.Anything).Return(nil, apperror.ErrNotFound)

		w := httptest.NewRecorder()
		body := `{"payment_method_id":"pm_test_123"}`
		r := httptest.NewRequest(http.MethodPost, "/api/v1/orders/"+orderID.String()+"/pay", strings.NewReader(body))
		r = setAuthContext(r, userID)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func setupMux(t *testing.T) (*http.ServeMux, *MockPaymentRetrier) {
	cmd := NewMockPaymentRetrier(t)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")

	authed.HandleFunc("POST /orders/{id}/pay", New(cmd, v).Retry)

	return mux, cmd
}

func setAuthContext(r *http.Request, userID uuid.UUID) *http.Request {
	ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
		UserID: userID,
		Email:  "test@example.com",
		Role:   "user",
	})
	return r.WithContext(ctx)
}
