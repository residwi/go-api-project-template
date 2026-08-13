package cart

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/cart/add"
	addpg "github.com/residwi/go-api-project-template/internal/modules/cart/add/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	"github.com/residwi/go-api-project-template/internal/modules/cart/empty"
	emptypg "github.com/residwi/go-api-project-template/internal/modules/cart/empty/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/lock"
	lockpg "github.com/residwi/go-api-project-template/internal/modules/cart/lock/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/query"
	querypg "github.com/residwi/go-api-project-template/internal/modules/cart/query/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/remove"
	removepg "github.com/residwi/go-api-project-template/internal/modules/cart/remove/postgres"
	"github.com/residwi/go-api-project-template/internal/modules/cart/updatequantity"
	updatequantitypg "github.com/residwi/go-api-project-template/internal/modules/cart/updatequantity/postgres"
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
	Query          *query.Reader
	Add            *add.Command
	UpdateQuantity *updatequantity.Command
	Remove         *remove.Command
	Lock           *lock.Command
	Empty          *empty.Command
}

func New(d Deps) *Module {
	return &Module{
		Query:          query.New(querypg.New(d.Pool), d.Products),
		Add:            add.New(addpg.New(d.Pool), d.Tx, d.Products, d.MaxItems),
		UpdateQuantity: updatequantity.New(updatequantitypg.New(d.Pool), d.Products),
		Remove:         remove.New(removepg.New(d.Pool)),
		Lock:           lock.New(lockpg.New(d.Pool)),
		Empty:          empty.New(emptypg.New(d.Pool)),
	}
}

func (m *Module) LockCart(ctx context.Context, userID uuid.UUID) error {
	return m.Lock.LockCart(ctx, userID)
}

func (m *Module) GetSnapshot(ctx context.Context, userID uuid.UUID) (*contract.Cart, error) {
	return m.Query.GetSnapshot(ctx, userID)
}

func (m *Module) Clear(ctx context.Context, userID uuid.UUID) error {
	return m.Empty.Clear(ctx, userID)
}
