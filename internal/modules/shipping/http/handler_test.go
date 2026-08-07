package http

import (
	"encoding/json"
	"maps"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/query"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/testhelper"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
)

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

func setupShippingMux(
	t *testing.T,
) (*http.ServeMux, *MockRepository, *MockOrderProvider, *MockOrderUpdater) {
	repo := NewMockRepository(t)
	orderProv := NewMockOrderProvider(t)
	orderUpd := NewMockOrderUpdater(t)
	svc := shipping.NewService(repo, testhelper.FakeTxRunner{}, orderProv, orderUpd)
	v := validator.New()

	mux := http.NewServeMux()
	authed := middleware.NewRouteGroup(mux, "/api/v1")
	admin := middleware.NewRouteGroup(mux, "/api/v1/admin")

	// repo and orderProv already satisfy query.Repository and query.OrderProvider
	// -- shipping.Shipment is a type alias for domain.Shipment and GetInfo's
	// signature matches exactly -- so the authed route's module needs no mock of
	// its own. Admin tests never exercise it, and nothing calls a method with no
	// expectation set.
	mod := &shipping.Module{Query: query.New(repo, orderProv)}

	RegisterRoutes(authed, admin, RouteDeps{
		Validator: v,
		Service:   svc,
		Module:    mod,
	})

	return mux, repo, orderProv, orderUpd
}
