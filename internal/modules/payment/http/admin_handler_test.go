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

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	mocks "github.com/residwi/go-api-project-template/mocks/payment"
)

func TestAdminHandler_List(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		mux, repo, _, _ := setupPaymentMux(t)

		now := time.Now()
		payments := []payment.Payment{
			{
				ID:        uuid.New(),
				OrderID:   uuid.New(),
				Amount:    money.New(5000, "USD"),
				Status:    payment.StatusSuccess,
				CreatedAt: now,
				UpdatedAt: now,
			},
		}

		repo.EXPECT().ListAdmin(mock.Anything, mock.MatchedBy(func(p payment.AdminListParams) bool {
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

		mux, repo, _, _ := setupPaymentMux(t)

		repo.EXPECT().ListAdmin(mock.Anything, mock.Anything).
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

		mux, repo, _, _ := setupPaymentMux(t)

		paymentID := uuid.New()
		now := time.Now()
		p := &payment.Payment{
			ID:        paymentID,
			OrderID:   uuid.New(),
			Amount:    money.New(10000, "USD"),
			Status:    payment.StatusSuccess,
			CreatedAt: now,
			UpdatedAt: now,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil)

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

		mux, _, _, _ := setupPaymentMux(t)

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

		mux, repo, _, _ := setupPaymentMux(t)

		paymentID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(nil, apperror.ErrNotFound)

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

		mux, repo, _, _ := setupPaymentMux(t)

		paymentID := uuid.New()
		orderID := uuid.New()
		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil)
		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *payment.Job) bool {
			return job.PaymentID == paymentID &&
				job.OrderID == orderID &&
				job.Action == payment.ActionRefund &&
				job.Status == payment.JobStatusPending
		})).Return(nil)

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

		mux, _, _, _ := setupPaymentMux(t)

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

		mux, repo, _, _ := setupPaymentMux(t)

		paymentID := uuid.New()
		p := &payment.Payment{
			ID:     paymentID,
			Status: payment.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/admin/payments/"+paymentID.String()+"/refund", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})
}

// payment.Payment.GatewayResponse is populated by service.go marshaling the
// gateway's ChargeResponse and persisting it via repo.UpdateGateway -- it
// carries no json:"-" tag, so toAdminPaymentResponse's explicit field list
// is the only thing keeping it off the wire.
func TestToAdminPaymentResponse_OmitsGatewayResponse(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	gatewayResponse := []byte(`{"card_number":"4242424242424242","cvv":"123"}`)

	got := toAdminPaymentResponse(&payment.Payment{
		ID:              uuid.New(),
		OrderID:         uuid.New(),
		Amount:          money.New(5000, "USD"),
		Status:          payment.StatusSuccess,
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

	// []byte marshals to base64, not plaintext, so a plaintext NotContains check
	// can never fire even if GatewayResponse were re-added to the DTO. Assert
	// against the base64 encoding instead so this check is actually capable of
	// catching that regression.
	assert.NotContains(t, string(raw), base64.StdEncoding.EncodeToString(gatewayResponse),
		"GatewayResponse may carry PII or card metadata and must never be serialised, even to an admin")
	assert.NotContains(t, string(raw), "gateway_response",
		"the GatewayResponse field must not appear under any key")
}

// setupPaymentMux returns only the mocks these tests actually drive. The rest
// are still constructed -- NewService needs all eight, and mockery's NewMockX(t)
// registers the expectation assertions -- they are just not handed back.
func setupPaymentMux(t *testing.T) (
	*http.ServeMux,
	*mocks.MockRepository,
	*mocks.MockOrderUpdater,
	*mocks.MockOrderGetter,
) {
	repo := mocks.NewMockRepository(t)
	gw := mocks.NewMockGateway(t)
	orders := mocks.NewMockOrderUpdater(t)
	orderGet := mocks.NewMockOrderGetter(t)
	orderItems := mocks.NewMockOrderItemsGetter(t)
	inv := mocks.NewMockInventoryDeductor(t)
	invRestore := mocks.NewMockInventoryRestorer(t)
	couponRel := mocks.NewMockCouponReleaser(t)

	svc := payment.NewService(repo, testhelper.FakeTxRunner{}, gw, orders, orderGet, orderItems, inv, invRestore, couponRel)
	v := validator.New()

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api")
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	RegisterRoutes(api, admin, RouteDeps{Validator: v, Service: svc})

	return mux, repo, orders, orderGet
}
