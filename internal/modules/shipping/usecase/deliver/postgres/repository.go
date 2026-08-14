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
	"github.com/residwi/go-api-project-template/internal/modules/shipping/usecase/deliver"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ deliver.Repository = (*Repository)(nil)

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

func (r *Repository) MarkDelivered(ctx context.Context, id uuid.UUID) (*domain.Shipment, error) {
	db := database.DB(ctx, r.pool)
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
