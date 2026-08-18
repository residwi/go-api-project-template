package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

type UseCase struct {
	repo   Repository
	orders OrderGetter
}

func New(repo Repository, orders OrderGetter) *UseCase {
	return &UseCase{repo: repo, orders: orders}
}

func (r *UseCase) GetByOrderIDForUser(ctx context.Context, userID, orderID uuid.UUID) (*domain.Shipment, error) {
	order, err := r.orders.GetInfo(ctx, orderID)
	if err != nil {
		return nil, err
	}

	if order.UserID != userID {
		return nil, apperror.ErrNotFound
	}

	return r.repo.GetByOrderID(ctx, orderID)
}
