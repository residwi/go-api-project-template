package updatetracking

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
)

type Params struct {
	Carrier        string
	TrackingNumber string
}

// Command takes no TxRunner: it changes fields on a row it already fetched
// and writes it back -- there is nothing outside itself to ask
// (ARCHITECTURE.md decision 14).
type Command struct{ repo Repository }

func New(repo Repository) *Command { return &Command{repo: repo} }

func (c *Command) Execute(ctx context.Context, shipmentID uuid.UUID, p Params) (*domain.Shipment, error) {
	shipment, err := c.repo.GetByID(ctx, shipmentID)
	if err != nil {
		return nil, err
	}

	// Empty means "leave it", not "blank it": the endpoint is a partial update.
	if p.Carrier != "" {
		shipment.Carrier = p.Carrier
	}
	if p.TrackingNumber != "" {
		shipment.TrackingNumber = p.TrackingNumber
	}

	if err := c.repo.Update(ctx, shipment); err != nil {
		return nil, err
	}

	return c.repo.GetByID(ctx, shipmentID)
}
