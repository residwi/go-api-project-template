package shipping

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/deliver"
	deliverpg "github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/deliver/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/updatetracking"
	updatetrackingpg "github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/updatetracking/postgres"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool *pgxpool.Pool
	Tx   database.TxRunner

	Orders OrderPorts
}

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
