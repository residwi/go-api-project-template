package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

type Reader struct {
	repo   Repository
	orders OrderPort
}

func New(repo Repository, orders OrderPort) *Reader {
	return &Reader{repo: repo, orders: orders}
}

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
