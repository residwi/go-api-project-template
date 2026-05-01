package shipping

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Repository interface {
	Create(ctx context.Context, shipment *Shipment) error
	GetByID(ctx context.Context, id uuid.UUID) (*Shipment, error)
	GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Shipment, error)
	Update(ctx context.Context, shipment *Shipment) error
	MarkShipped(ctx context.Context, id uuid.UUID) error
	MarkDelivered(ctx context.Context, id uuid.UUID) error
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) Create(ctx context.Context, shipment *Shipment) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO shipments (order_id, carrier, tracking_number, status)
		VALUES ($1, $2, $3, $4)
		RETURNING id, created_at, updated_at`,
		shipment.OrderID, shipment.Carrier, shipment.TrackingNumber, shipment.Status,
	).Scan(&shipment.ID, &shipment.CreatedAt, &shipment.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating shipment: %w", err)
	}
	return nil
}

func (r *PostgresRepository) GetByID(ctx context.Context, id uuid.UUID) (*Shipment, error) {
	db := database.DB(ctx, r.pool)
	var s Shipment
	err := db.QueryRow(ctx,
		`SELECT id, order_id, carrier, tracking_number, status, shipped_at, delivered_at, created_at, updated_at
		FROM shipments WHERE id = $1`, id,
	).Scan(&s.ID, &s.OrderID, &s.Carrier, &s.TrackingNumber, &s.Status,
		&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting shipment by id: %w", err)
	}
	return &s, nil
}

func (r *PostgresRepository) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*Shipment, error) {
	db := database.DB(ctx, r.pool)
	var s Shipment
	err := db.QueryRow(ctx,
		`SELECT id, order_id, carrier, tracking_number, status, shipped_at, delivered_at, created_at, updated_at
		FROM shipments WHERE order_id = $1`, orderID,
	).Scan(&s.ID, &s.OrderID, &s.Carrier, &s.TrackingNumber, &s.Status,
		&s.ShippedAt, &s.DeliveredAt, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, core.ErrNotFound
		}
		return nil, fmt.Errorf("getting shipment by order id: %w", err)
	}
	return &s, nil
}

func (r *PostgresRepository) Update(ctx context.Context, shipment *Shipment) error {
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
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkShipped(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE shipments SET status = 'shipped', shipped_at = NOW()
		WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("marking shipment as shipped: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (r *PostgresRepository) MarkDelivered(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`UPDATE shipments SET status = 'delivered', delivered_at = NOW()
		WHERE id = $1`, id,
	)
	if err != nil {
		return fmt.Errorf("marking shipment as delivered: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return core.ErrNotFound
	}
	return nil
}
