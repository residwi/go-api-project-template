package http

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/usecase/webhook"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

// These route-level tests wire a REAL webhook.Command -- not a mock of this
// package's own Command interface -- because the signature check they exist
// to prove lives inside Execute now. A mocked Command would let a forged
// signature through without either handler or command ever computing an
// HMAC, which is exactly the gap this test suite closes.

func TestWebhookHandler_SignatureVerification(t *testing.T) {
	t.Parallel()

	const secret = "whsec_test"
	sign := func(body []byte) string {
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		return hex.EncodeToString(mac.Sum(nil))
	}

	t.Run("valid signature is accepted", func(t *testing.T) {
		t.Parallel()

		// Unknown payment id: Execute no-ops and returns nil, which is enough to
		// prove the signature check passed and the body reached the command.
		mux := setupWebhookMux(t, secret, &fakeRepo{}, &fakeOrders{}, &fakeFinalizer{}, &fakeJobs{})

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
		t.Parallel()

		mux := setupWebhookMux(t, secret, &fakeRepo{}, &fakeOrders{}, &fakeFinalizer{}, &fakeJobs{})

		body, _ := json.Marshal(map[string]any{"event": "success"})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})

	t.Run("wrong signature is rejected", func(t *testing.T) {
		t.Parallel()

		mux := setupWebhookMux(t, secret, &fakeRepo{}, &fakeOrders{}, &fakeFinalizer{}, &fakeJobs{})

		body, _ := json.Marshal(map[string]any{"event": "success"})
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader(body))
		r.Header.Set("X-Webhook-Signature", "deadbeef")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)
	})
}

func TestWebhookHandler_HandleWebhook(t *testing.T) {
	t.Parallel()

	t.Run("success with valid JSON payload", func(t *testing.T) {
		t.Parallel()

		paymentID := uuid.New()
		orderID := uuid.New()
		p := &domain.Payment{ID: paymentID, OrderID: orderID, Status: domain.StatusPending}

		repo := &fakeRepo{getByID: func(uuid.UUID) (*domain.Payment, error) { return p, nil }}
		orders := &fakeOrders{}
		jobs := &fakeJobs{}
		mux := setupWebhookMux(t, "", repo, orders, &fakeFinalizer{}, jobs)

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
		assert.True(t, repo.updateStatusCalled)
		assert.True(t, repo.clearURLCalled)
		assert.True(t, jobs.cancelPendingCalled)
		// A failed payment also cancels the order and releases its reserved stock.
		assert.True(t, orders.cancelUnpaidCalled)
	})

	t.Run("invalid JSON returns 200", func(t *testing.T) {
		t.Parallel()

		mux := setupWebhookMux(t, "", &fakeRepo{}, &fakeOrders{}, &fakeFinalizer{}, &fakeJobs{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/payments/webhook", bytes.NewReader([]byte("not json")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("finalize failure runs compensating refund and returns 200", func(t *testing.T) {
		t.Parallel()

		// Funds are captured, so a finalize failure must not 5xx and leave the order
		// unpaid: the compensating refund runs and the webhook is acked with 200.
		paymentID := uuid.New()
		orderID := uuid.New()
		p := &domain.Payment{ID: paymentID, OrderID: orderID, Status: domain.StatusPending}

		repo := &fakeRepo{getByID: func(uuid.UUID) (*domain.Payment, error) { return p, nil }}
		finalizer := &fakeFinalizer{finalizeErr: errors.New("finalization failed")}
		mux := setupWebhookMux(t, "", repo, &fakeOrders{}, finalizer, &fakeJobs{})

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
		assert.True(t, finalizer.compensatingCalled)
	})
}

func setupWebhookMux(
	t *testing.T,
	secret string,
	repo *fakeRepo,
	orders *fakeOrders,
	finalizer *fakeFinalizer,
	jobs *fakeJobs,
) *http.ServeMux {
	t.Helper()

	cmd := webhook.New(repo, orders, finalizer, jobs, secret, testhelper.DiscardLogger())

	mux := http.NewServeMux()
	api := middleware.NewRouteGroup(mux, "/api")
	api.HandleFunc("POST /payments/webhook", New(cmd, testhelper.DiscardLogger()).HandleWebhook)

	return mux
}

// The four fakes below stand in for webhook's ports. They are hand-written,
// not mockery-generated: webhook's mocks are private to package webhook (a
// _test.go mock never leaves its package), so a route-level test in this
// package that wants a REAL webhook.Command -- to actually exercise
// signature verification -- cannot reach them.

type fakeRepo struct {
	getByID            func(uuid.UUID) (*domain.Payment, error)
	updateStatusCalled bool
	clearURLCalled     bool
}

func (f *fakeRepo) GetByID(_ context.Context, id uuid.UUID) (*domain.Payment, error) {
	if f.getByID != nil {
		return f.getByID(id)
	}
	return nil, apperror.ErrNotFound
}

func (f *fakeRepo) GetByGatewayTxnID(context.Context, string) (*domain.Payment, error) {
	return nil, apperror.ErrNotFound
}

func (f *fakeRepo) UpdateStatus(context.Context, uuid.UUID, domain.Status, []domain.Status) error {
	f.updateStatusCalled = true
	return nil
}

func (f *fakeRepo) ClearPaymentURL(context.Context, uuid.UUID) error {
	f.clearURLCalled = true
	return nil
}

type fakeOrders struct {
	cancelUnpaidCalled bool
}

func (f *fakeOrders) CancelUnpaid(context.Context, uuid.UUID) error {
	f.cancelUnpaidCalled = true
	return nil
}

type fakeFinalizer struct {
	finalizeErr        error
	compensatingCalled bool
}

func (f *fakeFinalizer) FinalizePaymentSuccess(context.Context, domain.Job) error {
	return f.finalizeErr
}

func (f *fakeFinalizer) RunCompensatingRefund(context.Context, domain.Job) {
	f.compensatingCalled = true
}

type fakeJobs struct {
	cancelPendingCalled bool
}

func (f *fakeJobs) CancelPendingByOrderID(context.Context, uuid.UUID) error {
	f.cancelPendingCalled = true
	return nil
}

func (f *fakeJobs) MarkJobCompletedByPaymentID(context.Context, uuid.UUID, domain.JobAction) error {
	return nil
}
