package http

import (
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/query"
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

		mux := setupQueryMux(t,
			fakeRepository{shipment: &domain.Shipment{
				ID:             shipmentID,
				OrderID:        orderID,
				Carrier:        "FedEx",
				TrackingNumber: "TRACK123",
				Status:         domain.StatusShipped,
				CreatedAt:      now,
				UpdatedAt:      now,
			}},
			fakeOrderProvider{order: ordercontract.Order{ID: orderID, UserID: userID, Status: "shipped"}},
		)

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

		mux := setupQueryMux(t, fakeRepository{}, fakeOrderProvider{})

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

		mux := setupQueryMux(t, fakeRepository{}, fakeOrderProvider{})

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

		orderID := uuid.New()
		mux := setupQueryMux(t, fakeRepository{}, fakeOrderProvider{err: apperror.ErrNotFound})

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		r = r.WithContext(middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: uuid.New(),
			Email:  "test@example.com",
			Role:   "user",
		}))

		mux.ServeHTTP(w, r)

		assert.Equal(t, http.StatusNotFound, w.Code)
	})

	t.Run("not owned by user", func(t *testing.T) {
		t.Parallel()

		userID := uuid.New()
		otherUserID := uuid.New()
		orderID := uuid.New()

		mux := setupQueryMux(t,
			fakeRepository{},
			fakeOrderProvider{order: ordercontract.Order{ID: orderID, UserID: otherUserID, Status: "shipped"}},
		)

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

		mux := setupQueryMux(t,
			fakeRepository{err: apperror.ErrNotFound},
			fakeOrderProvider{order: ordercontract.Order{ID: orderID, UserID: userID, Status: "shipped"}},
		)

		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/v1/orders/"+orderID.String()+"/shipping", nil)
		r = r.WithContext(middleware.SetUserContext(r.Context(), middleware.UserContext{
			UserID: userID, Email: "test@example.com", Role: "user",
		}))

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

// fakeRepository and fakeOrderProvider are hand-rolled rather than generated:
// query.Repository and query.OrderProvider are declared in query, so their
// mockery mocks live there too (query/mocks_test.go), private to that package.
// This package needs its own doubles to drive Handler through a real
// *query.Reader.
type fakeRepository struct {
	shipment *domain.Shipment
	err      error
}

func (f fakeRepository) GetByOrderID(context.Context, uuid.UUID) (*domain.Shipment, error) {
	return f.shipment, f.err
}

type fakeOrderProvider struct {
	order ordercontract.Order
	err   error
}

func (f fakeOrderProvider) GetInfo(context.Context, uuid.UUID) (ordercontract.Order, error) {
	return f.order, f.err
}

func setupQueryMux(t *testing.T, repo query.Repository, orders query.OrderProvider) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	New(query.New(repo, orders)).RegisterHTTP(authed)

	return mux
}
