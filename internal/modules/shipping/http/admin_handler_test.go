package http_test

import (
	"bytes"
	"context"
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

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_CreateShipment(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, orderProv, orderUpd := setupShippingMux(t)

		orderID := uuid.New()
		shipmentID := uuid.New()
		now := time.Now()

		orderProv.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{
			ID:     orderID,
			UserID: uuid.New(),
			Status: "paid",
		}, nil)
		repo.EXPECT().Create(mock.Anything, mock.Anything).
			Run(func(_ context.Context, s *shipping.Shipment) {
				s.ID = shipmentID
				s.CreatedAt = now
				s.UpdatedAt = now
			}).Return(nil)
		orderUpd.EXPECT().MarkShipped(mock.Anything, orderID).Return(nil)

		body, _ := json.Marshal(map[string]any{
			"carrier":         "FedEx",
			"tracking_number": "TRACK123",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/"+orderID.String()+"/ship", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusCreated, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			Carrier string `json:"carrier"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Carrier string `json:"carrier"`
		}{Carrier: "FedEx"}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _ := setupShippingMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/bad/ship", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mux, _, _, _ := setupShippingMux(t)

		orderID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/"+orderID.String()+"/ship", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		mux, _, _, _ := setupShippingMux(t)

		orderID := uuid.New()
		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/"+orderID.String()+"/ship", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		mux, _, orderProv, _ := setupShippingMux(t)

		orderID := uuid.New()
		orderProv.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{}, apperror.ErrNotFound)

		body, _ := json.Marshal(map[string]any{
			"carrier":         "FedEx",
			"tracking_number": "TRACK123",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/orders/"+orderID.String()+"/ship", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_UpdateTracking(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _, _ := setupShippingMux(t)

		shipmentID := uuid.New()
		orderID := uuid.New()
		now := time.Now()

		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(&shipping.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "OLD123",
			Status:         shipping.StatusShipped,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil).Once()
		repo.EXPECT().Update(mock.Anything, mock.Anything).Return(nil)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(&shipping.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "UPS",
			TrackingNumber: "NEW456",
			Status:         shipping.StatusShipped,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil).Once()

		body, _ := json.Marshal(map[string]any{
			"carrier":         "UPS",
			"tracking_number": "NEW456",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/shipments/"+shipmentID.String()+"/tracking", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusOK, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.True(t, resp.Success)

		dataJSON, err := json.Marshal(resp.Data)
		require.NoError(t, err)
		var got struct {
			Carrier        string `json:"carrier"`
			TrackingNumber string `json:"tracking_number"`
		}
		require.NoError(t, json.Unmarshal(dataJSON, &got))
		assert.Equal(t, struct {
			Carrier        string `json:"carrier"`
			TrackingNumber string `json:"tracking_number"`
		}{Carrier: "UPS", TrackingNumber: "NEW456"}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _ := setupShippingMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/shipments/bad/tracking", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		mux, _, _, _ := setupShippingMux(t)

		shipmentID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/shipments/"+shipmentID.String()+"/tracking", bytes.NewReader([]byte("{bad")))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		mux, _, _, _ := setupShippingMux(t)

		shipmentID := uuid.New()
		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/shipments/"+shipmentID.String()+"/tracking", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _, _ := setupShippingMux(t)

		shipmentID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, apperror.ErrNotFound)

		body, _ := json.Marshal(map[string]any{
			"carrier":         "UPS",
			"tracking_number": "TRACK789",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPut, "/api/v1/admin/shipments/"+shipmentID.String()+"/tracking", bytes.NewReader(body))
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestHandler_MarkDelivered(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, _, orderUpd := setupShippingMux(t)

		shipmentID := uuid.New()
		orderID := uuid.New()
		now := time.Now()

		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(&shipping.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         shipping.StatusShipped,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil).Once()
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).Return(&shipping.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         shipping.StatusDelivered,
			DeliveredAt:    &now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil)
		orderUpd.EXPECT().MarkDelivered(mock.Anything, orderID).Return(nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/shipments/"+shipmentID.String()+"/deliver", nil)

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
		}{Status: "delivered"}, got)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _ := setupShippingMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/shipments/bad/deliver", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		mux, repo, _, _ := setupShippingMux(t)

		shipmentID := uuid.New()
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/shipments/"+shipmentID.String()+"/deliver", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}
