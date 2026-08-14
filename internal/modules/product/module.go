package product

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/product/usecase/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product/usecase/images"
	imagespg "github.com/residwi/go-api-project-template/internal/modules/product/usecase/images/postgres"
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
	Query  *query.Reader
	Create *create.Command
	Update *update.Command
	Delete *remove.Command
	Images *images.Command
}

func New(d Deps) *Module {
	return &Module{
		Query:  query.New(querypg.New(d.Pool), d.InventoryReader),
		Create: create.New(createpg.New(d.Pool), d.InventoryRegistrar),
		Update: update.New(updatepg.New(d.Pool)),
		Delete: remove.New(removepg.New(d.Pool)),
		Images: images.New(imagespg.New(d.Pool), d.InventoryReader),
	}
}
