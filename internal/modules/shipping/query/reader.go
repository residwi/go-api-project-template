package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

type Reader struct {
	repo   Repository
	orders OrderProvider
}

func New(repo Repository, orders OrderProvider) *Reader {
	return &Reader{repo: repo, orders: orders}
}

// GetByOrderIDForUser answers only for the caller's own order. A mismatch is
// ErrNotFound rather than ErrForbidden: a 403 would confirm the order exists to
// someone who does not own it.
func (r *Reader) GetByOrderIDForUser(ctx context.Context, userID, orderID uuid.UUID) (*domain.Shipment, error) {
	order, err := r.orders.GetInfo(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if order.UserID != userID {
		return nil, apperror.ErrNotFound
	}

	return r.repo.GetByOrderID(ctx, orderID)
}
