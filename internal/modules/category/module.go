package category

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/category/create"
	createpg "github.com/residwi/go-api-project-template/internal/modules/category/create/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/category/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/category/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/category/update"
	updatepg "github.com/residwi/go-api-project-template/internal/modules/category/update/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool

	Products remove.ProductCounter
}

type Module struct {
	Query  *query.Reader
	Create *create.Command
	Update *update.Command
	Delete *remove.Command
}

func New(d Deps) *Module {
	return &Module{
		Query:  query.New(querypg.New(d.Pool)),
		Create: create.New(createpg.New(d.Pool)),
		Update: update.New(updatepg.New(d.Pool)),
		Delete: remove.New(removepg.New(d.Pool), d.Products),
	}
}
