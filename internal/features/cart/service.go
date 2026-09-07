package cart

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/features/cart/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/tracing"
)

type Service struct {
	repo         Repository
	tx           database.TxRunner
	products     ProductLookup
	maxCartItems int
	tracer       trace.Tracer
}

func New(repo Repository, tx database.TxRunner, products ProductLookup, maxItems int) *Service {
	return &Service{
		repo:         repo,
		tx:           tx,
		products:     products,
		maxCartItems: maxItems,
		tracer:       otel.Tracer("github.com/residwi/go-api-project-template/internal/features/cart"),
	}
}

func (s *Service) Add(ctx context.Context, userID, productID uuid.UUID, quantity int) (err error) {
	ctx, span := s.tracer.Start(ctx, "cart.Add")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	info, err := s.products.GetInfo(ctx, productID)
	if err != nil {
		return err
	}

	if info.Status != domain.StatusPublished {
		return fmt.Errorf("%w: product is not available", errs.ErrBadRequest)
	}
	if info.Available < quantity {
		return apperror.ErrInsufficientStock
	}

	return s.tx.Run(ctx, func(txCtx context.Context) error {
		cartID, err := s.repo.GetOrCreate(txCtx, userID)
		if err != nil {
			return err
		}

		count, hasItem, err := s.repo.CountAndHasItem(txCtx, cartID, productID)
		if err != nil {
			return err
		}
		if !hasItem && count >= s.maxCartItems {
			return fmt.Errorf("%w: cart cannot have more than %d items", errs.ErrBadRequest, s.maxCartItems)
		}

		return s.repo.AddItem(txCtx, cartID, productID, quantity)
	})
}

func (s *Service) Remove(ctx context.Context, userID, productID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "cart.Remove")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.RemoveItem(ctx, cartID, productID)
}

func (s *Service) UpdateQuantity(ctx context.Context, userID, productID uuid.UUID, quantity int) (err error) {
	ctx, span := s.tracer.Start(ctx, "cart.UpdateQuantity")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	info, err := s.products.GetInfo(ctx, productID)
	if err != nil {
		return err
	}
	if info.Status != domain.StatusPublished {
		return fmt.Errorf("%w: product is not available", errs.ErrBadRequest)
	}
	if info.Available < quantity {
		return apperror.ErrInsufficientStock
	}

	cartID, err := s.repo.GetOrCreate(ctx, userID)
	if err != nil {
		return err
	}

	return s.repo.UpdateItemQuantity(ctx, cartID, productID, quantity)
}

func (s *Service) Get(ctx context.Context, userID uuid.UUID) (_ *domain.Cart, err error) {
	ctx, span := s.tracer.Start(ctx, "cart.Get")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

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
			c.Items[i].Product = &domain.Product{Status: "unavailable"}
			continue
		}
		c.Items[i].Product = &domain.Product{
			Name:   info.Name,
			Price:  info.Price,
			Stock:  info.Available,
			Status: info.Status,
		}
	}
	return c, nil
}

func (s *Service) Snapshot(ctx context.Context, userID uuid.UUID) (_ *Snapshot, err error) {
	ctx, span := s.tracer.Start(ctx, "cart.Snapshot")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	c, err := s.Get(ctx, userID)
	if err != nil {
		return nil, err
	}

	snap := &Snapshot{ID: c.ID}
	for _, item := range c.Items {
		si := Item{ProductID: item.ProductID, Quantity: item.Quantity}
		if item.Product != nil {
			si.Name = item.Product.Name
			si.Price = item.Product.Price
			si.Status = item.Product.Status
		}
		snap.Items = append(snap.Items, si)
	}
	return snap, nil
}

func (s *Service) Clear(ctx context.Context, userID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "cart.Clear")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	return s.repo.Clear(ctx, userID)
}

func (s *Service) Lock(ctx context.Context, userID uuid.UUID) (err error) {
	ctx, span := s.tracer.Start(ctx, "cart.Lock")
	defer span.End()
	defer func() { tracing.Record(span, err) }()

	_, err = s.repo.GetCartForLock(ctx, userID)
	return err
}
