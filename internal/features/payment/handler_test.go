package payment_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/core/response"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	mocks "github.com/residwi/go-api-project-template/mocks/payment"
)

func setupPaymentMux(t *testing.T) (
	*http.ServeMux,
	*mocks.MockRepository,
	*mocks.MockGateway,
	*mocks.MockOrderUpdater,
	*mocks.MockOrderGetter,
) {
	repo := mocks.NewMockRepository(t)
	gw := mocks.NewMockGateway(t)
	orders := mocks.NewMockOrderUpdater(t)
	orderGet := mocks.NewMockOrderGetter(t)
	orderItems := mocks.NewMockOrderItemsGetter(t)
	inv := mocks.NewMockInventoryDeductor(t)
	invRel := mocks.NewMockInventoryReleaser(t)
	invRestock := mocks.NewMockInventoryRestocker(t)
	couponRel := mocks.NewMockCouponReleaser(t)

	svc := payment.NewService(repo, nil, gw, orders, orderGet, orderItems, inv, invRel, invRestock, couponRel)
	v := validator.New()

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api")
	admin := middleware.NewRouteGroup(mux, "/api/admin")
	payment.RegisterRoutes(api, admin, payment.RouteDeps{Validator: v, Service: svc})

	return mux, repo, gw, orders, orderGet
}

func TestWebhookHandler_HandleWebhook(t *testing.T) {
	t.Run("success with valid JSON payload", func(t *testing.T) {
		mux, repo, _, _, _ := setupPaymentMux(t)

		paymentID := uuid.New()
		orderID := uuid.New()
		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil)
		repo.EXPECT().UpdateStatus(mock.Anything, paymentID, payment.StatusCancelled,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing}).Return(nil)
		repo.EXPECT().ClearPaymentURL(mock.Anything, paymentID).Return(nil)
		repo.EXPECT().CancelJobsByOrderID(mock.Anything, orderID).Return(nil)

		payload := map[string]any{
			"event":          "failed",
			"transaction_id": "txn_123",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		}
		body, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("invalid JSON returns 200", func(t *testing.T) {
		mux, _, _, _, _ := setupPaymentMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader([]byte("not json")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("service HandleWebhook error returns 500", func(t *testing.T) {
		mux, repo, _, _, orderGet := setupPaymentMux(t)

		paymentID := uuid.New()
		orderID := uuid.New()
		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Amount:  5000,
			Status:  payment.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil).Times(2)
		orderGet.EXPECT().GetByID(mock.Anything, orderID).Return(payment.OrderSnapshot{
			TotalAmount: 9999,
			Currency:    "USD",
		}, nil)

		payload := map[string]any{
			"event": "success",
			"metadata": map[string]any{
				"payment_id": paymentID.String(),
			},
		}
		body, _ := json.Marshal(payload)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		ctx := database.WithTestTx(r.Context(), noopDBTX{})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_List(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _, _, _ := setupPaymentMux(t)

		now := time.Now()
		payments := []payment.Payment{
			{
				ID:        uuid.New(),
				OrderID:   uuid.New(),
				Amount:    5000,
				Currency:  "USD",
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
		assert.NotNil(t, data["pagination"])
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _, _, _ := setupPaymentMux(t)

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
	t.Run("success", func(t *testing.T) {
		mux, repo, _, _, _ := setupPaymentMux(t)

		paymentID := uuid.New()
		now := time.Now()
		p := &payment.Payment{
			ID:        paymentID,
			OrderID:   uuid.New(),
			Amount:    10000,
			Currency:  "USD",
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

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			ID       string `json:"id"`
			Currency string `json:"currency"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			ID       string `json:"id"`
			Currency string `json:"currency"`
		}{ID: paymentID.String(), Currency: "USD"}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _, _ := setupPaymentMux(t)

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
		mux, repo, _, _, _ := setupPaymentMux(t)

		paymentID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(nil, core.ErrNotFound)

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
	t.Run("success", func(t *testing.T) {
		mux, repo, _, _, orderGet := setupPaymentMux(t)

		paymentID := uuid.New()
		orderID := uuid.New()
		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  payment.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil)
		orderGet.EXPECT().GetByID(mock.Anything, orderID).Return(payment.OrderSnapshot{
			Status: "awaiting_payment",
		}, nil)
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
		mux, _, _, _, _ := setupPaymentMux(t)

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
		mux, repo, _, _, _ := setupPaymentMux(t)

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
