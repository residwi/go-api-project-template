package cart

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/cart/contract"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

const productStatusPublished = "published"

type Service struct {
	repo         Repository
	tx           database.TxRunner
	products     ProductLookup
	maxCartItems int
}

func NewService(repo Repository, tx database.TxRunner, products ProductLookup, maxCartItems int) *Service {
	return &Service{
		repo:         repo,
		tx:           tx,
		products:     products,
		maxCartItems: maxCartItems,
	}
}

func (s *Service) AddItem(ctx context.Context, userID uuid.UUID, p AddItemParams) error {
	info, err := s.products.GetInfo(ctx, p.ProductID)
	if err != nil {
		return err
	}

	if info.Status != productStatusPublished {
		return fmt.Errorf("%w: product is not available", apperror.ErrBadRequest)
	}
	if info.Available < p.Quantity {
		return apperror.ErrInsufficientStock
	}

	return s.tx.Run(ctx, func(txCtx context.Context) error {
		cartID, err := s.repo.GetOrCreate(txCtx, userID)
		if err != nil {
			return err
		}

		count, hasItem, err := s.repo.CountAndHasItem(txCtx, cartID, p.ProductID)
		if err != nil {
			return err
		}
		// Only a new distinct product can push the cart past the cap; bumping the
		// quantity of a product already in the cart is always allowed.
		if !hasItem && count >= s.maxCartItems {
			return fmt.Errorf("%w: cart cannot have more than %d items", apperror.ErrBadRequest, s.maxCartItems)
		}

		return s.repo.AddItem(txCtx, cartID, p.ProductID, p.Quantity)
	})
}

func (s *Service) RemoveItem(ctx context.Context, userID, productID uuid.UUID) error {
	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.RemoveItem(ctx, cartID, productID)
}

func (s *Service) UpdateQuantity(ctx context.Context, userID, productID uuid.UUID, p UpdateQuantityParams) error {
	// Mirrors AddItem's guards, or its stock check is bypassed by following it
	// with an UpdateQuantity.
	info, err := s.products.GetInfo(ctx, productID)
	if err != nil {
		return err
	}
	if info.Status != productStatusPublished {
		return fmt.Errorf("%w: product is not available", apperror.ErrBadRequest)
	}
	if info.Available < p.Quantity {
		return apperror.ErrInsufficientStock
	}

	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.UpdateItemQuantity(ctx, cartID, productID, p.Quantity)
}

// LockCart serializes concurrent checkouts of one cart. The order service calls
// it inside its PlaceOrder transaction.
func (s *Service) LockCart(ctx context.Context, userID uuid.UUID) error {
	_, err := s.repo.GetCartForLock(ctx, userID)
	return err
}

func (s *Service) GetCart(ctx context.Context, userID uuid.UUID) (*Cart, error) {
	c, err := s.repo.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(c.Items) == 0 {
		return c, nil
	}

	ids := make([]uuid.UUID, len(c.Items))
	for i := range c.Items {
		ids[i] = c.Items[i].ProductID
	}
	infos, err := s.products.GetInfoByIDs(ctx, ids)
	if err != nil {
		return nil, fmt.Errorf("looking up cart products: %w", err)
	}

	for i := range c.Items {
		info, ok := infos[c.Items[i].ProductID]
		if !ok {
			// Gone entirely: keep the line visible and unsellable rather than let the
			// customer's total change silently.
			c.Items[i].Product = &Product{Status: "unavailable"}
			continue
		}
		c.Items[i].Product = &Product{
			Name:   info.Name,
			Price:  info.Price,
			Stock:  info.Available,
			Status: info.Status,
		}
	}
	return c, nil
}

func (s *Service) Clear(ctx context.Context, userID uuid.UUID) error {
	return s.repo.Clear(ctx, userID)
}

// GetSnapshot freezes the cart for checkout. Order reads this instead of Cart so
// that cart's own model stays free of a checkout-shaped view.
func (s *Service) GetSnapshot(ctx context.Context, userID uuid.UUID) (*contract.Cart, error) {
	c, err := s.GetCart(ctx, userID)
	if err != nil {
		return nil, err
	}

	snap := &contract.Cart{ID: c.ID}
	for _, item := range c.Items {
		si := contract.CartItem{ProductID: item.ProductID, Quantity: item.Quantity}
		if item.Product != nil {
			si.Name = item.Product.Name
			si.Price = item.Product.Price
			si.Status = item.Product.Status
		}
		snap.Items = append(snap.Items, si)
	}
	return snap, nil
}
