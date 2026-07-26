package wiring

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/inventory"
	"github.com/residwi/go-api-project-template/internal/product"
)

// inventoryReaderAdapter maps product's Availability onto inventory's Stock.
// Note it drops Reserved: product asks what is sellable, not how much is held.
type inventoryReaderAdapter struct{ svc *inventory.Service }

func (a *inventoryReaderAdapter) GetAvailability(ctx context.Context, ids []uuid.UUID) (map[uuid.UUID]product.Availability, error) {
	levels, err := a.svc.GetLevels(ctx, ids)
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]product.Availability, len(levels))
	for id, st := range levels {
		out[id] = product.Availability{OnHand: st.Quantity, Available: st.Available}
	}
	return out, nil
}

func NewProductService(repo product.Repository, inventorySvc *inventory.Service) *product.Service {
	return product.NewService(repo, &inventoryReaderAdapter{svc: inventorySvc}, inventorySvc)
}
