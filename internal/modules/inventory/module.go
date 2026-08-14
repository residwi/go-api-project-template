package inventory

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/adjust"
	adjustpg "github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/adjust/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/deduct"
	deductpg "github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/deduct/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/register"
	registerpg "github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/register/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/reserve"
	reservepg "github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/reserve/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/restock"
	restockpg "github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/restock/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/restore"
	restorepg "github.com/residwi/go-api-project-template/internal/modules/inventory/usecase/restore/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool
}

type Module struct {
	Query    *query.Reader
	Restock  *restock.Command
	Adjust   *adjust.Command
	Reserve  *reserve.Command
	Deduct   *deduct.Command
	Register *register.Command

	restore *restore.Command
}

func New(d Deps) *Module {
	return &Module{
		Query:    query.New(querypg.New(d.Pool)),
		Restock:  restock.New(restockpg.New(d.Pool)),
		Adjust:   adjust.New(adjustpg.New(d.Pool)),
		Reserve:  reserve.New(reservepg.New(d.Pool)),
		Deduct:   deduct.New(deductpg.New(d.Pool)),
		Register: register.New(registerpg.New(d.Pool)),
		restore:  restore.New(restorepg.New(d.Pool)),
	}
}

func (m *Module) ReserveBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return m.Reserve.ReserveBatch(ctx, items)
}

func (m *Module) DeductBatch(ctx context.Context, items map[uuid.UUID]int) error {
	return m.Deduct.DeductBatch(ctx, items)
}

func (m *Module) Restore(ctx context.Context, items map[uuid.UUID]int, prior contract.StockState) error {
	return m.restore.Restore(ctx, items, prior)
}
