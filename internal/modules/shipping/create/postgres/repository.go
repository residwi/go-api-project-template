package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/create"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ create.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, shipment *domain.Shipment) error {
	db := database.DB(ctx, r.pool)

	var shippedAt *time.Time
	if shipment.Status == domain.StatusShipped {
		now := time.Now()
		shippedAt = &now
	}

	err := db.QueryRow(ctx,
		`INSERT INTO shipments (order_id, carrier, tracking_number, status, shipped_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, shipped_at, created_at, updated_at`,
		shipment.OrderID, shipment.Carrier, shipment.TrackingNumber, shipment.Status, shippedAt,
	).Scan(&shipment.ID, &shipment.ShippedAt, &shipment.CreatedAt, &shipment.UpdatedAt)
	if err != nil {
		if database.IsUniqueViolation(err) {
			return fmt.Errorf("%w: shipment already exists for this order", apperror.ErrConflict)
		}
		return fmt.Errorf("creating shipment: %w", err)
	}
	return nil
}
