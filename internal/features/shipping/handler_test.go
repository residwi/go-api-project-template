package shipping_test

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
	"github.com/residwi/go-api-project-template/internal/features/shipping"
	"github.com/residwi/go-api-project-template/internal/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	shipMocks "github.com/residwi/go-api-project-template/mocks/shipping"
)

func setupShippingMux(t *testing.T) (*http.ServeMux, *shipMocks.MockRepository, *shipMocks.MockOrderProvider, *shipMocks.MockOrderUpdater) {
	repo := shipMocks.NewMockRepository(t)
	orderProv := shipMocks.NewMockOrderProvider(t)
	orderUpd := shipMocks.NewMockOrderUpdater(t)
	svc := shipping.NewService(repo, orderProv, orderUpd)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	shipping.RegisterRoutes(authed, admin, shipping.RouteDeps{
		Validator: v,
		Service:   svc,
		Orders:    orderProv,
	})

	return mux, repo, orderProv, orderUpd
}

func TestHandler_GetShipping(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mux, repo, orderProv, _ := setupShippingMux(t)

		userID := uuid.New()
		orderID := uuid.New()
		shipmentID := uuid.New()
		now := time.Now()

		orderProv.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{
			ID:     orderID,
			UserID: userID,
			Status: "shipped",
		}, nil)

		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(&shipping.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         shipping.StatusShipped,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID,
			Email:  "test@example.com",
			Role:   "user",
		})
		r = r.WithContext(ctx)

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
		mux, _, _, _ := setupShippingMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+uuid.NewString()+"/shipping", nil)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusUnauthorized, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
	})

	t.Run("invalid UUID", func(t *testing.T) {
		mux, _, _, _ := setupShippingMux(t)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/bad/shipping", nil)
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: uuid.New(),
			Email:  "test@example.com",
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusBadRequest, w.Code)

		var resp response.Response
		require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
		assert.False(t, resp.Success)
		assert.Equal(t, "invalid id", resp.Error.Message)
	})

	t.Run("not found", func(t *testing.T) {
		mux, _, orderProv, _ := setupShippingMux(t)

		orderID := uuid.New()
		orderProv.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{}, core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: uuid.New(),
			Email:  "test@example.com",
			Role:   "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("not owned by user", func(t *testing.T) {
		mux, _, orderProv, _ := setupShippingMux(t)

		userID := uuid.New()
		otherUserID := uuid.New()
		orderID := uuid.New()

		orderProv.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{
			ID: orderID, UserID: otherUserID, Status: "shipped",
		}, nil)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("shipment service error", func(t *testing.T) {
		mux, repo, orderProv, _ := setupShippingMux(t)

		userID := uuid.New()
		orderID := uuid.New()

		orderProv.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{
			ID: orderID, UserID: userID, Status: "shipped",
		}, nil)
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(nil, core.ErrNotFound)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		ctx := middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		})
		r = r.WithContext(ctx)

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

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
		repo.EXPECT().Create(mock.Anything, mock.Anything).Return(nil)
		repo.EXPECT().MarkShipped(mock.Anything, mock.Anything).Return(nil)
		orderUpd.EXPECT().UpdateStatus(mock.Anything, orderID, []string{"paid", "processing"}, "shipped").Return(nil)
		repo.EXPECT().GetByID(mock.Anything, mock.Anything).Return(&shipping.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         shipping.StatusShipped,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil)

		body, _ := json.Marshal(shipping.CreateShipmentRequest{
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
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
		orderProv.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{}, core.ErrNotFound)

		body, _ := json.Marshal(shipping.CreateShipmentRequest{
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
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

		body, _ := json.Marshal(shipping.UpdateTrackingRequest{
			Carrier:        "UPS",
			TrackingNumber: "NEW456",
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
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(nil, core.ErrNotFound)

		body, _ := json.Marshal(shipping.UpdateTrackingRequest{
			Carrier:        "UPS",
			TrackingNumber: "TRACK789",
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
		repo.EXPECT().MarkDelivered(mock.Anything, shipmentID).Return(nil)
		orderUpd.EXPECT().UpdateStatus(mock.Anything, orderID, []string{"shipped"}, "delivered").Return(nil)
		repo.EXPECT().GetByID(mock.Anything, shipmentID).Return(&shipping.Shipment{
			ID:             shipmentID,
			OrderID:        orderID,
			Carrier:        "FedEx",
			TrackingNumber: "TRACK123",
			Status:         shipping.StatusDelivered,
			DeliveredAt:    &now,
			CreatedAt:      now,
			UpdatedAt:      now,
		}, nil).Once()

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
