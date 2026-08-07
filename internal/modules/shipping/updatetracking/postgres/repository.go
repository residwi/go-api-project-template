package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/updatetracking"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ updatetracking.Repository = (*Repository)(nil)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Shipment, error) {
	db := database.DB(ctx, r.pool)
	var s domain.Shipment
	err := db.QueryRow(ctx,
		`SELECT id, order_id, carrier, tracking_number, status, shipped_at, delivered_at, created_at, updated_at
		FROM shipments WHERE id = $1`, id,
	).Scan(&s.ID, &s.OrderID, &s.Carrier, &s.TrackingNumber, &s.Status,
		&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting shipment by id: %w", err)
	}
	return &s, nil
}

func (r *Repository) Update(ctx context.Context, shipment *domain.Shipment) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE shipments SET carrier=$1, tracking_number=$2, status=$3
		WHERE id = $4`,
		shipment.Carrier, shipment.TrackingNumber, shipment.Status, shipment.ID,
	)
	if err != nil {
		return fmt.Errorf("updating shipment: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}
