package product

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/product/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/product/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product/images"
	imagespg "github.com/residwi/go-api-project-template/internal/modules/product/images/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/product/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/product/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/product/update"
	updatepg "github.com/residwi/go-api-project-template/internal/modules/product/update/postgres"
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
