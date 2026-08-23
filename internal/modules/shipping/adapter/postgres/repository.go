package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/shipping"
	"github.com/residwi/go-api-project-template/internal/modules/shipping/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ shipping.Repository = (*Repository)(nil)

type Repository struct {
	db database.DB
}

func New(db database.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, shipment *domain.Shipment) error {
	db := database.PrimaryDB(ctx, r.db)

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

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Shipment, error) {
	db := database.PrimaryDB(ctx, r.db)
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

func (r *Repository) MarkDelivered(ctx context.Context, id uuid.UUID) (*domain.Shipment, error) {
	db := database.PrimaryDB(ctx, r.db)
	var s domain.Shipment
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

func (r *Repository) GetByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Shipment, error) {
	db := database.PrimaryDB(ctx, r.db)
	var s domain.Shipment
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

func (r *Repository) Update(ctx context.Context, shipment *domain.Shipment) error {
	db := database.PrimaryDB(ctx, r.db)
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
