package http

import (
	"bytes"
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
	"github.com/residwi/go-api-project-template/internal/modules/shipping/create"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func TestHandler_Create(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()
		shipmentID := uuid.New()
		now := time.Now()

		mux := setupCreateMux(t,
			fakeRepository{shipmentID: shipmentID, now: now},
			fakeOrderPort{order: ordercontract.Order{ID: orderID, UserID: uuid.New(), Status: "paid"}},
		)

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

		mux := setupCreateMux(t, fakeRepository{}, fakeOrderPort{})

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

		mux := setupCreateMux(t, fakeRepository{}, fakeOrderPort{})

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

		mux := setupCreateMux(t, fakeRepository{}, fakeOrderPort{})

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

	t.Run("command error", func(t *testing.T) {
		t.Parallel()

		orderID := uuid.New()

		mux := setupCreateMux(t, fakeRepository{}, fakeOrderPort{getInfoErr: apperror.ErrNotFound})

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

// fakeRepository and fakeOrderPort are hand-rolled rather than generated:
// create.Repository and create.OrderPort are declared in create, so their
// mockery mocks live there too (create/mocks_test.go), private to that
// package. This package needs its own doubles to drive Handler through a real
// *create.Command.
type fakeRepository struct {
	shipmentID uuid.UUID
	now        time.Time
	err        error
}

func (f fakeRepository) Create(_ context.Context, s *domain.Shipment) error {
	if f.err != nil {
		return f.err
	}
	s.ID = f.shipmentID
	s.CreatedAt = f.now
	s.UpdatedAt = f.now
	return nil
}

type fakeOrderPort struct {
	order       ordercontract.Order
	getInfoErr  error
	markShipErr error
}

func (f fakeOrderPort) GetInfo(context.Context, uuid.UUID) (ordercontract.Order, error) {
	return f.order, f.getInfoErr
}

func (f fakeOrderPort) MarkShipped(context.Context, uuid.UUID) error {
	return f.markShipErr
}

func setupCreateMux(t *testing.T, repo create.Repository, orders create.OrderPort) *http.ServeMux {
	t.Helper()

	mux := http.NewServeMux()
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")
	cmd := create.New(repo, testhelper.FakeTxRunner{}, orders)
	New(cmd, validator.New()).RegisterHTTP(admin)

	return mux
}
