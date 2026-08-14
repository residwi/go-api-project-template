package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Reader struct {
	repo Repository
}

func New(repo Repository) *Reader {
	return &Reader{repo: repo}
}

func (r *Reader) ListByUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Order, error) {
	return r.repo.ListByUser(ctx, userID, cursor)
}

func (r *Reader) GetByIDForUser(ctx context.Context, userID, orderID uuid.UUID) (*domain.Order, error) {
	order, err := r.getByID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	if order.UserID != userID {
		return nil, apperror.ErrNotFound
	}

	items, err := r.repo.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	order.Items = items

	return order, nil
}

func (r *Reader) ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Order, int, error) {
	return r.repo.ListAdmin(ctx, params)
}

func (r *Reader) GetByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	order, err := r.getByID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	items, err := r.repo.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}
	order.Items = items

	return order, nil
}

func (r *Reader) GetSnapshot(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	o, err := r.getByID(ctx, orderID)
	if err != nil {
		return contract.Order{}, err
	}

	couponCode := ""
	if o.CouponCode != nil {
		couponCode = *o.CouponCode
	}

	return contract.Order{
		Total:         o.Total,
		Status:        string(o.Status),
		CouponCode:    couponCode,
		StockDeducted: o.StockDeducted,
		StockReversed: o.StockReversed,
		Dispatched:    o.Dispatched(),
	}, nil
}

func (r *Reader) GetInfo(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	o, err := r.getByID(ctx, orderID)
	if err != nil {
		return contract.Order{}, err
	}
	return contract.Order{ID: o.ID, UserID: o.UserID, Status: string(o.Status)}, nil
}

func (r *Reader) ListItemQuantities(ctx context.Context, orderID uuid.UUID) (map[uuid.UUID]int, error) {
	items, err := r.repo.ListItemsByOrderID(ctx, orderID)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]int, len(items))
	for _, item := range items {
		out[item.ProductID] = item.Quantity
	}
	return out, nil
}

func (r *Reader) HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error) {
	return r.repo.HasDeliveredOrder(ctx, DeliveredPurchaseParams{
		UserID:    userID,
		OrderID:   orderID,
		ProductID: productID,
	})
}

func (r *Reader) getByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	return r.repo.GetByID(ctx, orderID)
}
