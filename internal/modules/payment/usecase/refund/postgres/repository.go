package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/apperror"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/usecase/refund"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ refund.Repository = (*Repository)(nil)

type amountColumns struct {
	amount   int64
	currency string
}

func (a amountColumns) assignTo(p *domain.Payment) {
	p.Amount = money.New(a.amount, a.currency)
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	db := database.DB(ctx, r.pool)
	var p domain.Payment
	var amt amountColumns
	var paymentMethodID, paymentURL, gatewayTxnID *string
	err := db.QueryRow(ctx,
		`SELECT id, order_id, amount, currency, status, method, payment_method_id, payment_url,
		        gateway_txn_id, gateway_response, paid_at, created_at, updated_at
		FROM payments WHERE id = $1`, id,
	).Scan(&p.ID, &p.OrderID, &amt.amount, &amt.currency, &p.Status, &p.Method,
		&paymentMethodID, &paymentURL, &gatewayTxnID, &p.GatewayResponse,
		&p.PaidAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, apperror.ErrNotFound
		}
		return nil, fmt.Errorf("getting payment by id: %w", err)
	}
	if paymentMethodID != nil {
		p.PaymentMethodID = *paymentMethodID
	}
	if paymentURL != nil {
		p.PaymentURL = *paymentURL
	}
	if gatewayTxnID != nil {
		p.GatewayTxnID = *gatewayTxnID
	}
	amt.assignTo(&p)
	return &p, nil
}

func (r *Repository) UpdateStatus(
	ctx context.Context,
	id uuid.UUID,
	toStatus domain.Status,
	fromStatuses []domain.Status,
) error {
	db := database.DB(ctx, r.pool)
	var returnedID uuid.UUID
	err := db.QueryRow(ctx,
		`UPDATE payments SET status = $1 WHERE id = $2 AND status = ANY($3) RETURNING id`,
		toStatus, id, fromStatuses,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("updating payment status: %w", err)
	}
	return nil
}
