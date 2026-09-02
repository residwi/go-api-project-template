package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/features/payment/domain"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
)

var _ payment.Repository = (*Repository)(nil)

type Repository struct {
	db database.DB
}

func New(db database.DB) *Repository {
	return &Repository{db: db}
}

func (r *Repository) Create(ctx context.Context, p *domain.Payment) error {
	db := database.PrimaryDB(ctx, r.db)
	err := db.QueryRow(
		ctx,
		`INSERT INTO payments (order_id, amount, currency, status, method, payment_method_id, payment_url, gateway_txn_id, gateway_response)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`,
		p.OrderID,
		p.Amount.Amount,
		p.Amount.Currency,
		p.Status,
		p.Method,
		nilIfEmpty(p.PaymentMethodID),
		nilIfEmpty(p.PaymentURL),
		nilIfEmpty(p.GatewayTxnID),
		p.GatewayResponse,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating payment: %w", err)
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	db := database.PrimaryDB(ctx, r.db)
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
			return nil, errs.ErrNotFound
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

func (r *Repository) GetActiveByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error) {
	db := database.PrimaryDB(ctx, r.db)
	var p domain.Payment
	var amt amountColumns
	var paymentMethodID, paymentURL, gatewayTxnID *string
	err := db.QueryRow(ctx,
		`SELECT id, order_id, amount, currency, status, method, payment_method_id, payment_url,
		        gateway_txn_id, gateway_response, paid_at, created_at, updated_at
		FROM payments WHERE order_id = $1 AND status IN ('pending', 'processing', 'requires_review')
		ORDER BY created_at DESC LIMIT 1`, orderID,
	).Scan(&p.ID, &p.OrderID, &amt.amount, &amt.currency, &p.Status, &p.Method,
		&paymentMethodID, &paymentURL, &gatewayTxnID, &p.GatewayResponse,
		&p.PaidAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errs.ErrNotFound
		}
		return nil, fmt.Errorf("getting active payment for order: %w", err)
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
	db := database.PrimaryDB(ctx, r.db)
	var returnedID uuid.UUID
	err := db.QueryRow(ctx,
		`UPDATE payments SET status = $1 WHERE id = $2 AND status = ANY($3) RETURNING id`,
		toStatus, id, fromStatuses,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errs.ErrConflict
		}
		return fmt.Errorf("updating payment status: %w", err)
	}
	return nil
}

func (r *Repository) UpdateGateway(
	ctx context.Context,
	id uuid.UUID,
	txnID string,
	response payment.GatewayChargeResponse,
) error {
	stored, err := json.Marshal(gatewayResponse{
		TransactionID: response.TransactionID,
		Status:        response.Status,
		PaymentURL:    response.PaymentURL,
	})
	if err != nil {
		return fmt.Errorf("marshaling gateway response: %w", err)
	}

	db := database.PrimaryDB(ctx, r.db)
	_, err = db.Exec(ctx,
		`UPDATE payments SET gateway_txn_id = $1, gateway_response = $2 WHERE id = $3`,
		txnID, stored, id,
	)
	if err != nil {
		return fmt.Errorf("updating gateway info: %w", err)
	}
	return nil
}

func (r *Repository) UpdatePaymentURL(ctx context.Context, id uuid.UUID, paymentURL string) error {
	db := database.PrimaryDB(ctx, r.db)
	_, err := db.Exec(ctx,
		`UPDATE payments SET payment_url = $1 WHERE id = $2`,
		paymentURL, id,
	)
	if err != nil {
		return fmt.Errorf("updating payment url: %w", err)
	}
	return nil
}

func (r *Repository) MarkPaid(ctx context.Context, id uuid.UUID, fromStatuses []domain.Status) error {
	db := database.PrimaryDB(ctx, r.db)
	var returnedID uuid.UUID
	err := db.QueryRow(ctx,
		`UPDATE payments SET status = 'success', paid_at = NOW() WHERE id = $1 AND status = ANY($2) RETURNING id`,
		id, fromStatuses,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errs.ErrConflict
		}
		return fmt.Errorf("marking payment paid: %w", err)
	}
	return nil
}

func (r *Repository) GetByGatewayTxnID(ctx context.Context, txnID string) (*domain.Payment, error) {
	db := database.PrimaryDB(ctx, r.db)
	var p domain.Payment
	var amt amountColumns
	var paymentMethodID, paymentURL, gwTxnID *string
	err := db.QueryRow(ctx,
		`SELECT id, order_id, amount, currency, status, method, payment_method_id, payment_url,
		        gateway_txn_id, gateway_response, paid_at, created_at, updated_at
		FROM payments WHERE gateway_txn_id = $1`, txnID,
	).Scan(&p.ID, &p.OrderID, &amt.amount, &amt.currency, &p.Status, &p.Method,
		&paymentMethodID, &paymentURL, &gwTxnID, &p.GatewayResponse,
		&p.PaidAt, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errs.ErrNotFound
		}
		return nil, fmt.Errorf("getting payment by gateway txn id: %w", err)
	}
	if paymentMethodID != nil {
		p.PaymentMethodID = *paymentMethodID
	}
	if paymentURL != nil {
		p.PaymentURL = *paymentURL
	}
	if gwTxnID != nil {
		p.GatewayTxnID = *gwTxnID
	}
	amt.assignTo(&p)
	return &p, nil
}

func (r *Repository) ClearPaymentURL(ctx context.Context, id uuid.UUID) error {
	db := database.PrimaryDB(ctx, r.db)
	_, err := db.Exec(ctx,
		`UPDATE payments SET payment_url = NULL WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("clearing payment url: %w", err)
	}
	return nil
}

func (r *Repository) ListAdmin(ctx context.Context, params payment.AdminListParams) ([]domain.Payment, int, error) {
	db := database.PrimaryDB(ctx, r.db)

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

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

type amountColumns struct {
	amount   int64
	currency string
}

func (a amountColumns) assignTo(p *domain.Payment) {
	p.Amount = money.New(a.amount, a.currency)
}
