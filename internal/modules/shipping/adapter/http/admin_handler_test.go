package http

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

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/web"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

func TestAdminHandler_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()
		shipmentID := uuid.New()
		now := time.Now()

		mux, service := setupAdminMux(t)
		service.EXPECT().
			Create(mock.Anything, orderID, "FedEx", "TRACK123").
			Return(&domain.Shipment{
				ID:             shipmentID,
				OrderID:        orderID,
				Carrier:        "FedEx",
				TrackingNumber: "TRACK123",
				Status:         domain.StatusShipped,
				CreatedAt:      now,
				UpdatedAt:      now,
			}, nil)

		body, _ := json.Marshal(map[string]any{
			"carrier":         "FedEx",
			"tracking_number": "TRACK123",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/admin/orders/"+orderID.String()+"/ship",
			bytes.NewReader(body),
		)
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
		t.Parallel()

		mux, _ := setupAdminMux(t)

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
		t.Parallel()

		mux, _ := setupAdminMux(t)

		orderID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/admin/orders/"+orderID.String()+"/ship",
			bytes.NewReader([]byte("{bad")),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		orderID := uuid.New()
		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/admin/orders/"+orderID.String()+"/ship",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()

		mux, service := setupAdminMux(t)
		service.EXPECT().
			Create(mock.Anything, orderID, "FedEx", "TRACK123").
			Return(nil, errs.ErrNotFound)

		body, _ := json.Marshal(map[string]any{
			"carrier":         "FedEx",
			"tracking_number": "TRACK123",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPost,
			"/api/v1/admin/orders/"+orderID.String()+"/ship",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func TestAdminHandler_Deliver(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()
		orderID := uuid.New()
		now := time.Now()

		mux, service := setupAdminMux(t)
		service.EXPECT().Deliver(mock.Anything, shipmentID).Return(&domain.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         domain.StatusDelivered,
			DeliveredAt:    &now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil)

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
		t.Parallel()

		mux, _ := setupAdminMux(t)

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
		t.Parallel()

		shipmentID := uuid.New()

		mux, service := setupAdminMux(t)
		service.EXPECT().Deliver(mock.Anything, shipmentID).Return(nil, errors.New("db error"))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/shipments/"+shipmentID.String()+"/deliver", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestAdminHandler_UpdateTracking(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()
		orderID := uuid.New()
		now := time.Now()

		mux, service := setupAdminMux(t)
		service.EXPECT().
			UpdateTracking(mock.Anything, shipmentID, "UPS", "NEW456").
			Return(&domain.Shipment{
				ID:             shipmentID,
				OrderID:        orderID,
				Carrier:        "UPS",
				TrackingNumber: "NEW456",
				Status:         domain.StatusShipped,
				CreatedAt:      now,
				UpdatedAt:      now,
			}, nil)

		body, _ := json.Marshal(map[string]any{
			"carrier":         "UPS",
			"tracking_number": "NEW456",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/shipments/"+shipmentID.String()+"/tracking",
			bytes.NewReader(body),
		)
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
		t.Parallel()

		mux, _ := setupAdminMux(t)

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
		t.Parallel()

		mux, _ := setupAdminMux(t)

		shipmentID := uuid.New()

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/shipments/"+shipmentID.String()+"/tracking",
			bytes.NewReader([]byte("{bad")),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)
	})

	t.Run("validation error missing fields", func(t *testing.T) {
		t.Parallel()

		mux, _ := setupAdminMux(t)

		shipmentID := uuid.New()
		body, _ := json.Marshal(map[string]string{})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/shipments/"+shipmentID.String()+"/tracking",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnprocessableEntity, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "validation failed", resp.Error.Message)
	})

	t.Run("service error", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()

		mux, service := setupAdminMux(t)
		service.EXPECT().
			UpdateTracking(mock.Anything, shipmentID, "UPS", "TRACK789").
			Return(nil, errs.ErrNotFound)

		body, _ := json.Marshal(map[string]any{
			"carrier":         "UPS",
			"tracking_number": "TRACK789",
		})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodPut,
			"/api/v1/admin/shipments/"+shipmentID.String()+"/tracking",
			bytes.NewReader(body),
		)
		r.Header.Set("Content-Type", "application/json")

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

func setupAdminMux(t *testing.T) (*http.ServeMux, *MockShipmentManager) {
	t.Helper()

	service := NewMockShipmentManager(t)

	mux := http.NewServeMux()
	admin := web.NewRouter(mux).Group("/api/v1/admin")
	h := NewAdminHandler(service)
	admin.HandleFunc("POST /orders/{id}/ship", h.Create)
	admin.HandleFunc("PUT /shipments/{id}/tracking", h.UpdateTracking)
	admin.HandleFunc("POST /shipments/{id}/deliver", h.Deliver)

	return mux, service
}
