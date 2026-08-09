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
	"github.com/residwi/go-api-project-template/internal/modules/payment/query"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ query.Repository = (*Repository)(nil)

// Every read goes through this, so a scanned row becomes a denominated
// money.Money in one place instead of at each query.
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

func (r *Repository) ListAdmin(ctx context.Context, params query.AdminListParams) ([]domain.Payment, int, error) {
	db := database.DB(ctx, r.pool)

	where := "1=1"
	args := []any{}
	argIdx := 1

	if params.Status != "" {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, params.Status)
		argIdx++
	}
	if params.OrderID != "" {
		orderID, err := uuid.Parse(params.OrderID)
		if err == nil {
			where += fmt.Sprintf(" AND order_id = $%d", argIdx)
			args = append(args, orderID)
			argIdx++
		}
	}

	var total int
	if err := db.QueryRow(ctx, "SELECT COUNT(*) FROM payments WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("counting payments: %w", err)
	}

	q := fmt.Sprintf(
		`SELECT id, order_id, amount, currency, status, method, payment_method_id, gateway_txn_id, paid_at, created_at, updated_at
		FROM payments WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where,
		argIdx,
		argIdx+1,
	)
	args = append(args, params.Limit(), params.Offset())

	rows, err := db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing payments: %w", err)
	}

	payments, err := pgx.CollectRows(rows, scanPaymentAdmin)
	if err != nil {
		return nil, 0, fmt.Errorf("iterating payments: %w", err)
	}
	return payments, total, nil
}

func scanPaymentAdmin(row pgx.CollectableRow) (domain.Payment, error) {
	var p domain.Payment
	var amt amountColumns
	var paymentMethodID, gatewayTxnID *string
	if err := row.Scan(&p.ID, &p.OrderID, &amt.amount, &amt.currency, &p.Status, &p.Method,
		&paymentMethodID, &gatewayTxnID, &p.PaidAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return domain.Payment{}, fmt.Errorf("scanning payment: %w", err)
	}
	if paymentMethodID != nil {
		p.PaymentMethodID = *paymentMethodID
	}
	if gatewayTxnID != nil {
		p.GatewayTxnID = *gatewayTxnID
	}
	amt.assignTo(&p)
	return p, nil
}
