package http

import (
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
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Get(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		orderID := uuid.New()
		shipmentID := uuid.New()
		now := time.Now()

		service := NewMockShipmentReader(t)
		service.EXPECT().GetForUser(mock.Anything, userID, orderID).Return(&domain.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         domain.StatusShipped,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil)

		mux := setupMux(t, service)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		r = r.WithContext(middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID,
			Email:  "test@example.com",
			Role:   "user",
		}))

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
		}{Carrier: "FedEx", TrackingNumber: "TRACK123"}, got)
	})

	t.Run("missing auth", func(t *testing.T) {
		t.Parallel()

		mux := setupMux(t, NewMockShipmentReader(t))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+uuid.NewString()+"/shipping", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		t.Parallel()

		mux := setupMux(t, NewMockShipmentReader(t))

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/bad/shipping", nil)
		r = r.WithContext(middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: uuid.New(),
			Email:  "test@example.com",
			Role:   "user",
		}))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("not found", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		orderID := uuid.New()

		service := NewMockShipmentReader(t)
		service.EXPECT().GetForUser(mock.Anything, userID, orderID).Return(nil, apperror.ErrNotFound)

		mux := setupMux(t, service)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		r = r.WithContext(middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		}))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	// Service.GetForUser turns an ownership mismatch into apperror.ErrNotFound
	// itself (see service_test.go) -- from this handler's side of the port,
	// that looks identical to any other not-found.
	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		orderID := uuid.New()

		service := NewMockShipmentReader(t)
		service.EXPECT().GetForUser(mock.Anything, userID, orderID).Return(nil, apperror.ErrNotFound)

		mux := setupMux(t, service)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		r = r.WithContext(middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		}))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("shipment lookup error", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		orderID := uuid.New()

		service := NewMockShipmentReader(t)
		service.EXPECT().GetForUser(mock.Anything, userID, orderID).Return(nil, apperror.ErrNotFound)

		mux := setupMux(t, service)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		r = r.WithContext(middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		}))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

// toShipmentResponse is shared by handler.go and admin_handler.go. The old
// create, deliver, query and updatetracking handlers each carried their own
// copy of this exact test; three of the four (create, query, updatetracking)
// were byte-for-byte identical, and deliver's differed only in exercising
// delivered_at instead of leaving both pointer fields nil. Both scenarios
// survive here as subtests so neither's coverage is lost in the merge.
func TestToShipmentResponse_ExposesExactFieldSet(t *testing.T) {
	t.Parallel()

	t.Run("omits shipped_at and delivered_at when nil", func(t *testing.T) {
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
	})

	t.Run("includes delivered_at when set", func(t *testing.T) {
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
			[]string{
				"id", "order_id", "carrier", "tracking_number", "status", "delivered_at", "created_at", "updated_at",
			},
			slices.Collect(maps.Keys(fields)),
			"shipped_at is omitempty and absent when nil")
	})
}

func setupMux(t *testing.T, service ShipmentReader) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	authed.HandleFunc("GET /orders/{id}/shipping", NewHandler(service).Get)

	return mux
}
