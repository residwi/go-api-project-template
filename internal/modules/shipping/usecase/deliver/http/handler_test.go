package http

import (
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

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Deliver(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()
		orderID := uuid.New()
		now := time.Now()

		usecase := NewMockShipmentDeliverer(t)
		usecase.EXPECT().Execute(mock.Anything, shipmentID).Return(&domain.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         domain.StatusDelivered,
			DeliveredAt:    &now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil)

		mux := setupDeliverMux(t, usecase)

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

		mux := setupDeliverMux(t, NewMockShipmentDeliverer(t))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/shipments/bad/deliver", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("command error", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()

		usecase := NewMockShipmentDeliverer(t)
		usecase.EXPECT().Execute(mock.Anything, shipmentID).Return(nil, errors.New("db error"))

		mux := setupDeliverMux(t, usecase)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodPost, "/api/v1/admin/shipments/"+shipmentID.String()+"/deliver", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusInternalServerError, w.Code)
	})
}

func TestToShipmentResponse_ExposesExactFieldSet(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := toShipmentResponse(&domain.Shipment{
		ID:             uuid.New(),
		OrderID:        uuid.New(),
		Carrier:        "FedEx",
		TrackingNumber: "TRACK123",
		Status:         domain.StatusDelivered,
		DeliveredAt:    &now,
		CreatedAt:      now,
		UpdatedAt:      now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{"id", "order_id", "carrier", "tracking_number", "status", "delivered_at", "created_at", "updated_at"},
		slices.Collect(maps.Keys(fields)),
		"shipped_at is omitempty and absent when nil")
}

func setupDeliverMux(t *testing.T, usecase ShipmentDeliverer) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")
	admin.HandleFunc("POST /shipments/{id}/deliver", New(usecase).Deliver)

	return mux
}
