package cart

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
)

const productStatusPublished = "published"

type ProductLookup interface {
	GetByID(ctx context.Context, id uuid.UUID) (*ProductInfo, error)
}

type ProductInfo struct {
	ID       uuid.UUID
	Name     string
	Price    int64
	Currency string
	Status   string
}

type StockChecker interface {
	GetStock(ctx context.Context, productID uuid.UUID) (StockInfo, error)
}

type StockInfo struct {
	Available int
}

type Service struct {
	repo         Repository
	pool         *pgxpool.Pool
	products     ProductLookup
	stock        StockChecker
	maxCartItems int
}

func NewService(repo Repository, pool *pgxpool.Pool, products ProductLookup, stock StockChecker, maxCartItems int) *Service {
	return &Service{
		repo:         repo,
		pool:         pool,
		products:     products,
		stock:        stock,
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

	stockInfo, err := s.stock.GetStock(ctx, req.ProductID)
	if err != nil {
		return err
	}
	if stockInfo.Available < req.Quantity {
		return core.ErrInsufficientStock
	}

	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	count, err := s.repo.CountItems(ctx, cartID)
	if err != nil {
		return err
	}
	if count >= s.maxCartItems {
		return fmt.Errorf("%w: cart cannot have more than %d items", core.ErrBadRequest, s.maxCartItems)
	}

	return s.repo.AddItem(ctx, cartID, req.ProductID, req.Quantity)
}

func (s *Service) RemoveItem(ctx context.Context, userID, productID uuid.UUID) error {
	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.RemoveItem(ctx, cartID, productID)
}

func (s *Service) UpdateQuantity(ctx context.Context, userID, productID uuid.UUID, req UpdateItemRequest) error {
	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.UpdateItemQuantity(ctx, cartID, productID, req.Quantity)
}

func (s *Service) GetCart(ctx context.Context, userID uuid.UUID) (*Cart, error) {
	return s.repo.GetCart(ctx, userID)
}

func (s *Service) Clear(ctx context.Context, userID uuid.UUID) error {
	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.Clear(ctx, cartID)
}
