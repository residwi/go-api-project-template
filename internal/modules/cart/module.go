package cart

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/add"
	addpg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/add/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/clear"
	clearpg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/clear/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/lock"
	lockpg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/lock/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/usecase/updatequantity"
	updatequantitypg "github.com/residwi/go-api-project-template/internal/modules/cart/usecase/updatequantity/postgres"
	productcontract "github.com/residwi/go-api-project-template/internal/modules/product/contract"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Deps struct {
	Pool     *pgxpool.Pool
	Tx       database.TxRunner
	MaxItems int
	Products ProductPorts
}

type ProductPorts interface {
	GetInfo(ctx context.Context, id uuid.UUID) (*productcontract.Product, error)
	GetInfoByIDs(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]productcontract.Product, error)
}

type Module struct {
	Query          *query.UseCase
	Add            *add.UseCase
	UpdateQuantity *updatequantity.UseCase
	Remove         *remove.UseCase
	Lock           *lock.UseCase
	ClearCart      *clear.UseCase
}

func New(d Deps) *Module {
	return &Module{
		Query:          query.New(querypg.New(d.Pool), d.Products),
		Add:            add.New(addpg.New(d.Pool), d.Tx, d.Products, d.MaxItems),
		UpdateQuantity: updatequantity.New(updatequantitypg.New(d.Pool), d.Products),
		Remove:         remove.New(removepg.New(d.Pool)),
		Lock:           lock.New(lockpg.New(d.Pool)),
		ClearCart:      clear.New(clearpg.New(d.Pool)),
	}
}

func (m *Module) LockCart(ctx context.Context, userID uuid.UUID) error {
	return m.Lock.LockCart(ctx, userID)
}

func (m *Module) GetSnapshot(ctx context.Context, userID uuid.UUID) (*contract.Cart, error) {
	return m.Query.GetSnapshot(ctx, userID)
}

func (m *Module) Clear(ctx context.Context, userID uuid.UUID) error {
	return m.ClearCart.Clear(ctx, userID)
}
