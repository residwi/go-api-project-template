package wishlist

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/usecase/add"
	addpg "github.com/residwi/go-api-project-template/internal/modules/wishlist/usecase/add/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/wishlist/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/wishlist/usecase/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/wishlist/usecase/remove/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool
}

type Module struct {
	Query      *query.UseCase
	AddItem    *add.UseCase
	RemoveItem *remove.UseCase
}

func New(d Deps) *Module {
	return &Module{
		Query:      query.New(querypg.New(d.Pool)),
		AddItem:    add.New(addpg.New(d.Pool)),
		RemoveItem: remove.New(removepg.New(d.Pool)),
	}
}
