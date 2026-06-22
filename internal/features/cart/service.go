package cart

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const productStatusPublished = "published"

type ProductLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*ProductInfo, error)
}

type ProductInfo struct {
	ID        uuid.UUID
	Name      string
	Price     int64
	Currency  string
	Status    string
	Available int
}

type Service struct {
	repo         Repository
	pool         *pgxpool.Pool
	products     ProductLookup
	maxCartItems int
}

func NewService(repo Repository, pool *pgxpool.Pool, products ProductLookup, maxCartItems int) *Service {
	return &Service{
		repo:         repo,
		pool:         pool,
		products:     products,
		maxCartItems: maxCartItems,
	}
}

func (s *Service) AddItem(ctx context.Context, userID uuid.UUID, req AddItemRequest) error {
	p, err := s.products.GetByID(ctx, req.ProductID)
	if err != nil {
		return err
	}

	if p.Status != productStatusPublished {
		return fmt.Errorf("%w: product is not available", core.ErrBadRequest)
	}
	if p.Available < req.Quantity {
		return core.ErrInsufficientStock
	}

	return database.WithTx(ctx, s.pool, func(txCtx context.Context) error {
		cartID, err := s.repo.GetOrCreate(txCtx, userID)
		if err != nil {
			return err
		}

		count, hasItem, err := s.repo.CountAndHasItem(txCtx, cartID, req.ProductID)
		if err != nil {
			return err
		}
		// Only a new distinct product can push the cart past the cap; bumping the
		// quantity of a product already in the cart is always allowed.
		if !hasItem && count >= s.maxCartItems {
			return fmt.Errorf("%w: cart cannot have more than %d items", core.ErrBadRequest, s.maxCartItems)
		}

		return s.repo.AddItem(txCtx, cartID, req.ProductID, req.Quantity)
	})
}

func (s *Service) RemoveItem(ctx context.Context, userID, productID uuid.UUID) error {
	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.RemoveItem(ctx, cartID, productID)
}

func (s *Service) UpdateQuantity(ctx context.Context, userID, productID uuid.UUID, req UpdateItemRequest) error {
	// Mirror AddItem's guards: setting a quantity must respect product
	// availability and published status, otherwise AddItem's stock check is
	// trivially bypassed by following it with an UpdateQuantity.
	p, err := s.products.GetByID(ctx, productID)
	if err != nil {
		return err
	}
	if p.Status != productStatusPublished {
		return fmt.Errorf("%w: product is not available", core.ErrBadRequest)
	}
	if p.Available < req.Quantity {
		return core.ErrInsufficientStock
	}

	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.UpdateItemQuantity(ctx, cartID, productID, req.Quantity)
}

// LockCart takes a row lock on the user's cart for the current transaction so
// concurrent checkouts of the same cart serialize. It is used by the order
// service inside its PlaceOrder transaction.
func (s *Service) LockCart(ctx context.Context, userID uuid.UUID) error {
	_, err := s.repo.GetCartForLock(ctx, userID)
	return err
}

func (s *Service) GetCart(ctx context.Context, userID uuid.UUID) (*Cart, error) {
	return s.repo.GetCart(ctx, userID)
}

func (s *Service) Clear(ctx context.Context, userID uuid.UUID) error {
	return s.repo.Clear(ctx, userID)
}
