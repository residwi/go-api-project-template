package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
)

type Reader struct {
	repo Repository
}

func New(repo Repository) *Reader {
	return &Reader{repo: repo}
}

func (r *Reader) GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error) {
	return r.repo.GetStock(ctx, productID)
}

func (r *Reader) GetAvailability(
	ctx context.Context,
	ids []uuid.UUID,
) (map[uuid.UUID]contract.Availability, error) {
	levels, err := r.repo.GetLevels(ctx, ids)
	if err != nil {
		return nil, err
	}

	out := make(map[uuid.UUID]contract.Availability, len(levels))
	for id, st := range levels {
		out[id] = contract.Availability{OnHand: st.Quantity, Available: st.Available}
	}
	return out, nil
}
