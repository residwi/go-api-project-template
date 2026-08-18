package inventory

import (
	"github.com/jackc/pgx/v5/pgxpool"

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
	Query    *query.UseCase
	Restock  *restock.UseCase
	Adjust   *adjust.UseCase
	Reserve  *reserve.UseCase
	Deduct   *deduct.UseCase
	Register *register.UseCase
	Restore  *restore.UseCase
}

func New(d Deps) *Module {
	return &Module{
		Query:    query.New(querypg.New(d.Pool)),
		Restock:  restock.New(restockpg.New(d.Pool)),
		Adjust:   adjust.New(adjustpg.New(d.Pool)),
		Reserve:  reserve.New(reservepg.New(d.Pool)),
		Deduct:   deduct.New(deductpg.New(d.Pool)),
		Register: register.New(registerpg.New(d.Pool)),
		Restore:  restore.New(restorepg.New(d.Pool)),
	}
}
