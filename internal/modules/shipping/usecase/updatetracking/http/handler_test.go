package http

import (
	"bytes"
	"encoding/json"
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
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/updatetracking"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_UpdateTracking(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()
		orderID := uuid.New()
		now := time.Now()

		usecase := NewMockTrackingUpdater(t)
		usecase.EXPECT().
			Execute(mock.Anything, shipmentID, updatetracking.Params{Carrier: "UPS", TrackingNumber: "NEW456"}).
			Return(&domain.Shipment{
				ID:             shipmentID,
				OrderID:        orderID,
				Carrier:        "UPS",
				TrackingNumber: "NEW456",
				Status:         domain.StatusShipped,
				CreatedAt:      now,
				UpdatedAt:      now,
			}, nil)

		mux := setupUpdateTrackingMux(t, usecase)

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

		mux := setupUpdateTrackingMux(t, NewMockTrackingUpdater(t))

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

		mux := setupUpdateTrackingMux(t, NewMockTrackingUpdater(t))

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

		mux := setupUpdateTrackingMux(t, NewMockTrackingUpdater(t))

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

	t.Run("command error", func(t *testing.T) {
		t.Parallel()

		shipmentID := uuid.New()

		usecase := NewMockTrackingUpdater(t)
		usecase.EXPECT().
			Execute(mock.Anything, shipmentID, updatetracking.Params{Carrier: "UPS", TrackingNumber: "TRACK789"}).
			Return(nil, apperror.ErrNotFound)

		mux := setupUpdateTrackingMux(t, usecase)

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

func TestToShipmentResponse_ExposesExactFieldSet(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := toShipmentResponse(&domain.Shipment{
		ID:             uuid.New(),
		OrderID:        uuid.New(),
		Carrier:        "FedEx",
		TrackingNumber: "TRACK123",
		Status:         domain.StatusShipped,
		CreatedAt:      now,
		UpdatedAt:      now,
	})

	raw, err := json.Marshal(got)
	require.NoError(t, err)

	var fields map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(raw, &fields))
	assert.ElementsMatch(t,
		[]string{"id", "order_id", "carrier", "tracking_number", "status", "created_at", "updated_at"},
		slices.Collect(maps.Keys(fields)),
		"shipped_at and delivered_at are omitempty and absent when nil")
}

func setupUpdateTrackingMux(t *testing.T, usecase TrackingUpdater) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")
	admin.HandleFunc("PUT /shipments/{id}/tracking", New(usecase, validator.New()).UpdateTracking)

	return mux
}
