package http

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	mocks "github.com/residwi/go-api-project-template/mocks/payment"
)

func TestWebhookHandler_SignatureVerification(t *testing.T) {
	const secret = "whsec_test"
	sign := func(body []byte) string {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		return hex.EncodeToString(mac.Sum(nil))
	}

	t.Run("valid signature is accepted", func(t *testing.T) {
		mux, repo := setupPaymentMuxWithSecret(t, secret)
		// Unknown payment id: service no-ops and returns 200, which is enough to
		// prove the signature check passed and the body reached the service.
		repo.EXPECT().GetByID(mock.Anything, mock.Anything).Return(nil, apperror.ErrNotFound)

		body, _ := json.Marshal(map[string]any{
			"event":    "success",
			"metadata": map[string]any{"payment_id": uuid.New().String()},
		})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))
		r.Header.Set("X-Webhook-Signature", sign(body))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("missing signature is rejected", func(t *testing.T) {
		mux, _ := setupPaymentMuxWithSecret(t, secret)

		body, _ := json.Marshal(map[string]any{"event": "success"})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("wrong signature is rejected", func(t *testing.T) {
		mux, _ := setupPaymentMuxWithSecret(t, secret)

		body, _ := json.Marshal(map[string]any{"event": "success"})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))
		r.Header.Set("X-Webhook-Signature", "deadbeef")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestWebhookHandler_HandleWebhook(t *testing.T) {
	t.Run("success with valid JSON payload", func(t *testing.T) {
		mux, repo, orders, _ := setupPaymentMux(t)

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
		// Failed payment now also cancels the order and releases its reserved stock.
		orders.EXPECT().CancelUnpaid(mock.Anything, orderID).Return(nil)

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
		mux, _, _, _ := setupPaymentMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader([]byte("not json")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("finalize failure runs compensating refund and returns 200", func(t *testing.T) {
		// The gateway has already captured funds, so a finalize failure (here an
		// amount mismatch) must not 5xx and leave money captured with the order
		// unpaid. HandleWebhook now runs a compensating refund and acks the webhook
		// with 200 so the gateway stops retrying into an already-handled failure.
		mux, repo, orders, orderGet := setupPaymentMux(t)

		paymentID := uuid.New()
		orderID := uuid.New()
		p := &payment.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Amount:  money.New(5000, "USD"),
			Status:  payment.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).Return(p, nil).Times(2)
		orderGet.EXPECT().GetByID(mock.Anything, orderID).Return(payment.OrderSnapshot{
			Total: money.New(9999, "USD"),
		}, nil)

		// Compensating refund: flag payment requires_review, mark the order
		// fulfillment-failed, and enqueue a refund job.
		repo.EXPECT().UpdateStatus(mock.Anything, paymentID, payment.StatusRequiresReview,
			[]payment.Status{payment.StatusPending, payment.StatusProcessing, payment.StatusSuccess}).Return(nil)
		orders.EXPECT().MarkFulfillmentFailedCompensating(mock.Anything, orderID).Return(nil)
		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *payment.Job) bool {
			return job.PaymentID == paymentID &&
				job.OrderID == orderID &&
				job.Action == payment.ActionRefund
		})).Return(nil)

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

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})
}

func setupPaymentMuxWithSecret(t *testing.T, secret string) (*http.ServeMux, *mocks.MockRepository) {
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
	RegisterRoutes(api, admin, RouteDeps{Validator: v, Service: svc, WebhookSecret: secret})

	return mux, repo
}
