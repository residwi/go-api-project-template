package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/shipping"
)

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, shipment *shipping.Shipment) error {
	db := database.DB(ctx, r.pool)

	var shippedAt *time.Time
	if shipment.Status == shipping.StatusShipped {
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

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*shipping.Shipment, error) {
	db := database.DB(ctx, r.pool)
	var s shipping.Shipment
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

func (r *Repository) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*shipping.Shipment, error) {
	db := database.DB(ctx, r.pool)
	var s shipping.Shipment
	err := db.QueryRow(ctx,
		`SELECT id, order_id, carrier, tracking_number, status, shipped_at, delivered_at, created_at, updated_at
		FROM shipments WHERE order_id = $1`, orderID,
	).Scan(&s.ID, &s.OrderID, &s.Carrier, &s.TrackingNumber, &s.Status,
		&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting shipment by order id: %w", err)
	}
	return &s, nil
}

func (r *Repository) Update(ctx context.Context, shipment *shipping.Shipment) error {
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

func (r *Repository) MarkShipped(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE shipments SET status = 'shipped', shipped_at = NOW()
		WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("marking shipment as shipped: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return apperror.ErrNotFound
	}
	return nil
}

// MarkDelivered flips the shipment to delivered and returns the updated row in
// the same round-trip (RETURNING), so callers don't need a follow-up GetByID.
func (r *Repository) MarkDelivered(ctx context.Context, id uuid.UUID) (*shipping.Shipment, error) {
	db := database.DB(ctx, r.pool)
	var s shipping.Shipment
	err := db.QueryRow(ctx,
		`UPDATE shipments SET status = 'delivered', delivered_at = NOW()
		WHERE id = $1
		RETURNING id, order_id, carrier, tracking_number, status, shipped_at, delivered_at, created_at, updated_at`,
		id,
	).Scan(&s.ID, &s.OrderID, &s.Carrier, &s.TrackingNumber, &s.Status,
		&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("marking shipment as delivered: %w", err)
	}
	return &s, nil
}
