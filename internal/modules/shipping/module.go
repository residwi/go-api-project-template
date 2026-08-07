// Package shipping composes shipping's slices. It imports no transport package,
// so a worker or a future grpc server can construct this module without linking
// HTTP.
package shipping

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/shipping/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/deliver"
	deliverpg "github.com/residwi/go-api-project-template/internal/modules/shipping/deliver/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/shipping/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/updatetracking"
	updatetrackingpg "github.com/residwi/go-api-project-template/internal/modules/shipping/updatetracking/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool *pgxpool.Pool
	Tx   database.TxRunner

	// Orders is order's service. It satisfies each slice's own port by name-match,
	// so no adapter stands between them.
	Orders OrderPorts
}

// OrderPorts is the union of what shipping's slices need from order. Each slice
// still declares its own narrow port; this exists so Deps has one field instead
// of one per slice.
type OrderPorts interface {
	GetInfo(ctx context.Context, orderID uuid.UUID) (ordercontract.Order, error)
	MarkShipped(ctx context.Context, orderID uuid.UUID) error
	MarkDelivered(ctx context.Context, orderID uuid.UUID) error
}

type Module struct {
	Query          *query.Reader
	Create         *create.Command
	UpdateTracking *updatetracking.Command
	Deliver        *deliver.Command
}

func New(d Deps) *Module {
	return &Module{
		Query:          query.New(querypg.New(d.Pool), d.Orders),
		Create:         create.New(createpg.New(d.Pool), d.Tx, d.Orders),
		UpdateTracking: updatetracking.New(updatetrackingpg.New(d.Pool)),
		Deliver:        deliver.New(deliverpg.New(d.Pool), d.Tx, d.Orders),
	}
}
