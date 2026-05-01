package mock_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/platform/payment"
	mockgw "github.com/residwi/go-api-project-template/internal/platform/payment/mock"
)

func TestGateway_Charge(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := payment.ChargeResponse{
			TransactionID: "txn_123",
			Status:        "success",
			PaymentURL:    "https://example.com/pay",
		}

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/charge", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req payment.ChargeRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&req)) {
				return
			}
			assert.Equal(t, "order_1", req.OrderID)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(want)
		}))
		defer ts.Close()

		gw := mockgw.New(ts.URL, 5*time.Second)
		got, err := gw.Charge(context.Background(), payment.ChargeRequest{
			IdempotencyKey:  "key_1",
			OrderID:         "order_1",
			Amount:          5000,
			Currency:        "USD",
			PaymentMethodID: "pm_test",
		})

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("server error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		gw := mockgw.New(ts.URL, 5*time.Second)
		_, err := gw.Charge(context.Background(), payment.ChargeRequest{
			OrderID: "order_1",
			Amount:  5000,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("invalid json", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("not valid json"))
		}))
		defer ts.Close()

		gw := mockgw.New(ts.URL, 5*time.Second)
		_, err := gw.Charge(context.Background(), payment.ChargeRequest{
			OrderID: "order_1",
			Amount:  5000,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "decoding charge response")
	})

	t.Run("connection error", func(t *testing.T) {
		gw := mockgw.New("http://127.0.0.1:1", 1*time.Second)
		_, err := gw.Charge(context.Background(), payment.ChargeRequest{
			OrderID: "order_1",
			Amount:  5000,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "calling charge endpoint")
	})

	t.Run("invalid URL returns request creation error", func(t *testing.T) {
		gw := mockgw.New("http://invalid\x7furl", 1*time.Second)
		_, err := gw.Charge(context.Background(), payment.ChargeRequest{
			OrderID: "order_1",
			Amount:  5000,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating charge request")
	})
}

func TestGateway_Refund(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := payment.RefundResponse{
			RefundID: "ref_456",
			Status:   "success",
		}

		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			assert.Equal(t, "/refund", r.URL.Path)
			assert.Equal(t, http.MethodPost, r.Method)
			assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

			var req payment.RefundRequest
			if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&req)) {
				return
			}
			assert.Equal(t, "txn_123", req.TransactionID)

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(want)
		}))
		defer ts.Close()

		gw := mockgw.New(ts.URL, 5*time.Second)
		got, err := gw.Refund(context.Background(), payment.RefundRequest{
			TransactionID: "txn_123",
			Amount:        2500,
			Reason:        "customer request",
		})

		require.NoError(t, err)
		assert.Equal(t, want, got)
	})

	t.Run("server error", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer ts.Close()

		gw := mockgw.New(ts.URL, 5*time.Second)
		_, err := gw.Refund(context.Background(), payment.RefundRequest{
			TransactionID: "txn_123",
			Amount:        2500,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "500")
	})

	t.Run("invalid json response", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte("not valid json"))
		}))
		defer ts.Close()

		gw := mockgw.New(ts.URL, 5*time.Second)
		_, err := gw.Refund(context.Background(), payment.RefundRequest{
			TransactionID: "txn_123",
			Amount:        2500,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "decoding refund response")
	})

	t.Run("connection error", func(t *testing.T) {
		gw := mockgw.New("http://127.0.0.1:1", 1*time.Second)
		_, err := gw.Refund(context.Background(), payment.RefundRequest{
			TransactionID: "txn_123",
			Amount:        2500,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "calling refund endpoint")
	})

	t.Run("invalid URL returns request creation error", func(t *testing.T) {
		gw := mockgw.New("http://invalid\x7furl", 1*time.Second)
		_, err := gw.Refund(context.Background(), payment.RefundRequest{
			TransactionID: "txn_123",
			Amount:        2500,
		})

		require.Error(t, err)
		assert.Contains(t, err.Error(), "creating refund request")
	})
}
