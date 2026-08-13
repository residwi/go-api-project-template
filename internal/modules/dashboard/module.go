package dashboard

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/revenue"
	revenuepg "github.com/residwi/go-api-project-template/internal/modules/dashboard/revenue/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/summary"
	summarypg "github.com/residwi/go-api-project-template/internal/modules/dashboard/summary/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/topproducts"
	topproductspg "github.com/residwi/go-api-project-template/internal/modules/dashboard/topproducts/postgres"
)

type Deps struct {
	Pool *pgxpool.Pool
}

type Module struct {
	Summary     *summary.Reader
	TopProducts *topproducts.Reader
	Revenue     *revenue.Reader
}

func New(d Deps) *Module {
	return &Module{
		Summary:     summary.New(summarypg.New(d.Pool)),
		TopProducts: topproducts.New(topproductspg.New(d.Pool)),
		Revenue:     revenue.New(revenuepg.New(d.Pool)),
	}
}
