package http

import (
	"encoding/base64"
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

	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/features/payment/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestAdminHandler_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		now := time.Now()
		payments := []domain.Payment{
			{
				ID:        uuid.New(),
				OrderID:   uuid.New(),
				Amount:    money.New(5000, "USD"),
				Status:    domain.StatusSuccess,
				CreatedAt: now,
				UpdatedAt: now,
			},
		}

		service.EXPECT().ListAdmin(mock.Anything, mock.MatchedBy(func(p payment.AdminListParams) bool {
			return p.Page == 1 && p.PageSize == 20
		})).Return(payments, 1, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/payments", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		data, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		items, ok := data["items"].([]any)
		require.True(t, ok)
		assert.Len(t, items, 1)

		item := items[0].(map[string]any)
		assert.InDelta(t, float64(5000), item["amount"], 0.0001)
		assert.Equal(t, "USD", item["currency"])
		assert.Equal(t, "success", item["status"])
		assert.NotEmpty(t, item["id"])
		assert.NotEmpty(t, item["order_id"])
		assert.NotEmpty(t, item["created_at"])
		assert.NotEmpty(t, item["updated_at"])

		pagination, ok := data["pagination"].(map[string]any)
		require.True(t, ok)
		assert.InDelta(t, float64(1), pagination["current_page"], 0.0001)
		assert.InDelta(t, float64(20), pagination["page_size"], 0.0001)
		assert.InDelta(t, float64(1), pagination["total_items"], 0.0001)
		assert.InDelta(t, float64(1), pagination["total_pages"], 0.0001)
		assert.Equal(t, false, pagination["has_previous"])
		assert.Equal(t, false, pagination["has_next"])
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		service.EXPECT().ListAdmin(mock.Anything, mock.Anything).
			Return(nil, 0, errors.New("db connection failed"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/payments", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

func TestAdminHandler_Get(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		paymentID := uuid.New()
		now := time.Now()
		p := &domain.Payment{
			ID:        paymentID,
			OrderID:   uuid.New(),
			Amount:    money.New(10000, "USD"),
			Status:    domain.StatusSuccess,
			CreatedAt: now,
			UpdatedAt: now,
		}

		service.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/payments/"+paymentID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		obj, ok := resp.Data.(map[string]any)
		require.True(t, ok)
		// id is echoed back from the request, so it is deterministic.
		assert.Equal(t, paymentID.String(), obj["id"])
		assert.InDelta(t, float64(10000), obj["amount"], 0.0001)
		assert.Equal(t, "USD", obj["currency"])
		assert.Equal(t, "success", obj["status"])
		assert.NotEmpty(t, obj["order_id"])
		assert.NotEmpty(t, obj["created_at"])
		assert.NotEmpty(t, obj["updated_at"])
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/payments/not-a-uuid", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		paymentID := uuid.New()
		service.EXPECT().GetByID(mock.Anything, paymentID).Return(nil, errs.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/admin/payments/"+paymentID.String(), nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

func TestAdminHandler_Refund(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		paymentID := uuid.New()

		service.EXPECT().Refund(mock.Anything, paymentID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/admin/payments/"+paymentID.String()+"/refund", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			Status string `json:"status"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Status string `json:"status"`
		}{Status: "refund_enqueued"}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/admin/payments/not-a-uuid/refund", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("payment not refundable", func(t *testing.T) {
		t.Parallel()

		mux, service := setupAdminMux(t)

		paymentID := uuid.New()

		service.EXPECT().Refund(mock.Anything, paymentID).
			Return(errs.ErrBadRequest)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/admin/payments/"+paymentID.String()+"/refund", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

func TestToAdminPaymentResponse_OmitsGatewayResponse(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	gatewayResponse := []byte(`{"card_number":"4242424242424242","cvv":"123"}`)

	got := toAdminPaymentResponse(&domain.Payment{
		ID:              uuid.New(),
		OrderID:         uuid.New(),
		Amount:          money.New(5000, "USD"),
		Status:          domain.StatusSuccess,
		Method:          "card",
		PaymentMethodID: "pm_test_123",
		GatewayTxnID:    "txn_123",
		GatewayResponse: gatewayResponse,
		CreatedAt:       now,
		UpdatedAt:       now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{
			"id", "order_id", "amount", "currency", "status", "method", "payment_method_id",
			"gateway_txn_id", "created_at", "updated_at",
		},
		slices.Collect(maps.Keys(fields)),
		"payment_url and paid_at are omitempty and absent when unset; every other field must be present -- "+
			"this key-set assertion is the real control against GatewayResponse leaking back in, since it is a "+
			"[]byte and would marshal to base64 rather than the plaintext checked below")

	// []byte marshals to base64, so a plaintext NotContains could never fire even if
	// GatewayResponse came back. Assert the base64 form.
	assert.NotContains(t, string(raw), base64.StdEncoding.EncodeToString(gatewayResponse),
		"GatewayResponse may carry PII or card metadata and must never be serialised, even to an admin")
	assert.NotContains(t, string(raw), "gateway_response",
		"the GatewayResponse field must not appear under any key")
}

func setupAdminMux(t *testing.T) (*http.ServeMux, *MockPaymentManager) {
	service := NewMockPaymentManager(t)

	mux := http.NewServeMux()
	admin := web.NewRouter(mux).Group("/api/admin")
	h := NewAdminHandler(service)
	admin.HandleFunc("GET /payments", h.ListAdmin)
	admin.HandleFunc("GET /payments/{id}", h.GetByID)
	admin.HandleFunc("POST /payments/{id}/refund", h.Refund)

	return mux, service
}
