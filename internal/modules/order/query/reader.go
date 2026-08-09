// Package query serves every read order publishes: the customer's own orders,
// the admin listing, and the four cross-module reads payment, shipping and
// review consume by name-match.
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

// GetByIDForUser 404s an order that exists but belongs to someone else, rather
// than leaking a 403 that would confirm the id is valid.
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

// GetByID is the admin read: no ownership check, but still fills Items, unlike
// the bare getByID this and GetByIDForUser both build on.
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

// GetSnapshot backs payment's OrderGetter: everything payment needs to decide
// a charge or refund outcome, without payment importing order.
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

// GetInfo backs shipping's per-slice ownership checks, which need only who
// owns the order and its current status.
func (r *Reader) GetInfo(ctx context.Context, orderID uuid.UUID) (contract.Order, error) {
	o, err := r.getByID(ctx, orderID)
	if err != nil {
		return contract.Order{}, err
	}
	return contract.Order{ID: o.ID, UserID: o.UserID, Status: string(o.Status)}, nil
}

// ListItemQuantities backs payment's OrderItemsGetter. A paid order has one
// order line per product, so this cannot collide two items into the same key.
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

// HasDeliveredOrder backs review's PurchaseVerifier, so review confirms a
// purchase through this module instead of querying the orders schema.
func (r *Reader) HasDeliveredOrder(ctx context.Context, userID, orderID, productID uuid.UUID) (bool, error) {
	return r.repo.HasDeliveredOrder(ctx, DeliveredPurchaseParams{
		UserID:    userID,
		OrderID:   orderID,
		ProductID: productID,
	})
}

// getByID is the bare read GetSnapshot and GetInfo build on: no ownership
// check, no items -- callers that need either add it themselves.
func (r *Reader) getByID(ctx context.Context, orderID uuid.UUID) (*domain.Order, error) {
	return r.repo.GetByID(ctx, orderID)
}
