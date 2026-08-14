package dashboard

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/usecase/revenue"
	revenuepg "github.com/residwi/go-api-project-template/internal/modules/dashboard/usecase/revenue/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/usecase/summary"
	summarypg "github.com/residwi/go-api-project-template/internal/modules/dashboard/usecase/summary/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/usecase/topproducts"
	topproductspg "github.com/residwi/go-api-project-template/internal/modules/dashboard/usecase/topproducts/postgres"
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
