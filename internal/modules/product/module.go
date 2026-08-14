package product

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/product/usecase/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/manageimages"
	manageimagespg "github.com/residwi/go-api-project-template/internal/modules/product/usecase/manageimages/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/product/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/product/usecase/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/update"
	updatepg "github.com/residwi/go-api-project-template/internal/modules/product/usecase/update/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool

	InventoryReader    query.InventoryReader
	InventoryRegistrar create.InventoryRegistrar
}

type Module struct {
	Query  *query.UseCase
	Create *create.UseCase
	Update *update.UseCase
	Delete *remove.UseCase
	Images *manageimages.UseCase
}

func New(d Deps) *Module {
	return &Module{
		Query:  query.New(querypg.New(d.Pool), d.InventoryReader),
		Create: create.New(createpg.New(d.Pool), d.InventoryRegistrar),
		Update: update.New(updatepg.New(d.Pool)),
		Delete: remove.New(removepg.New(d.Pool)),
		Images: manageimages.New(manageimagespg.New(d.Pool)),
	}
}
