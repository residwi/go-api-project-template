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
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	shipMocks "github.com/residwi/go-api-project-template/mocks/shipping"
)

func TestHandler_GetShipping(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

		mux, _, orderProv, _ := setupShippingMux(t)

		orderID := uuid.New()
		orderProv.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{}, apperror.ErrNotFound)

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
		t.Parallel()

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
		t.Parallel()

		mux, repo, orderProv, _ := setupShippingMux(t)

		userID := uuid.New()
		orderID := uuid.New()

		orderProv.EXPECT().GetByID(mock.Anything, orderID).Return(shipping.OrderInfo{
			ID: orderID, UserID: userID, Status: "shipped",
		}, nil)
		repo.EXPECT().GetByOrderID(mock.Anything, orderID).Return(nil, apperror.ErrNotFound)

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

// None of this package's handler-level tests decode more than a couple of
// shipmentResponse's fields (see TestHandler_GetShipping above and
// admin_handler_test.go's TestAdminHandler_* cases), so this ElementsMatch is
// the only assertion in the package pinning the full wire shape.
func TestToShipmentResponse_ExposesExactFieldSet(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	got := toShipmentResponse(&shipping.Shipment{
		ID:             uuid.New(),
		OrderID:        uuid.New(),
		Carrier:        "FedEx",
		TrackingNumber: "TRACK123",
		Status:         shipping.StatusShipped,
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

func setupShippingMux(t *testing.T) (*http.ServeMux, *shipMocks.MockRepository, *shipMocks.MockOrderProvider, *shipMocks.MockOrderUpdater) {
	repo := shipMocks.NewMockRepository(t)
	orderProv := shipMocks.NewMockOrderProvider(t)
	orderUpd := shipMocks.NewMockOrderUpdater(t)
	svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orderProv, orderUpd)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	RegisterRoutes(authed, admin, RouteDeps{
		Validator: v,
		Service:   svc,
		Orders:    orderProv,
	})

	return mux, repo, orderProv, orderUpd
}
