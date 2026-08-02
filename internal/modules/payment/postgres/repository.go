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
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

// amountColumns is the payments table's amount and currency columns. Every read
// of a payment goes through it, so the one place a scanned row becomes a
// denominated money.Money is here rather than repeated at each of the five
// queries that select them.
type amountColumns struct {
	amount   int64
	currency string
}

func (a amountColumns) assignTo(p *payment.Payment) {
	p.Amount = money.New(a.amount, a.currency)
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) Create(ctx context.Context, p *payment.Payment) error {
	db := database.DB(ctx, r.pool)
	err := db.QueryRow(ctx,
		`INSERT INTO payments (order_id, amount, currency, status, method, payment_method_id, payment_url, gateway_txn_id, gateway_response)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id, created_at, updated_at`,
		p.OrderID, p.Amount.Amount, p.Amount.Currency, p.Status, p.Method,
		nilIfEmpty(p.PaymentMethodID), nilIfEmpty(p.PaymentURL),
		nilIfEmpty(p.GatewayTxnID), p.GatewayResponse,
	).Scan(&p.ID, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating payment: %w", err)
	}
	return nil
}

func (r *Repository) GetByID(ctx context.Context, id uuid.UUID) (*payment.Payment, error) {
	db := database.DB(ctx, r.pool)
	var p payment.Payment
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

func (r *Repository) GetActiveByOrderID(ctx context.Context, orderID uuid.UUID) (*payment.Payment, error) {
	db := database.DB(ctx, r.pool)
	var p payment.Payment
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
			return nil, apperror.ErrNotFound
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

func (r *Repository) GetByGatewayTxnID(ctx context.Context, txnID string) (*payment.Payment, error) {
	db := database.DB(ctx, r.pool)
	var p payment.Payment
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
			return nil, apperror.ErrNotFound
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

func (r *Repository) UpdateStatus(ctx context.Context, id uuid.UUID, toStatus payment.Status, fromStatuses []payment.Status) error {
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

func (r *Repository) UpdateGateway(ctx context.Context, id uuid.UUID, txnID string, response []byte) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payments SET gateway_txn_id = $1, gateway_response = $2 WHERE id = $3`,
		txnID, response, id,
	)
	if err != nil {
		return fmt.Errorf("updating gateway info: %w", err)
	}
	return nil
}

func (r *Repository) UpdatePaymentURL(ctx context.Context, id uuid.UUID, paymentURL string) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payments SET payment_url = $1 WHERE id = $2`,
		paymentURL, id,
	)
	if err != nil {
		return fmt.Errorf("updating payment url: %w", err)
	}
	return nil
}

func (r *Repository) ClearPaymentURL(ctx context.Context, id uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payments SET payment_url = NULL WHERE id = $1`,
		id,
	)
	if err != nil {
		return fmt.Errorf("clearing payment url: %w", err)
	}
	return nil
}

func (r *Repository) MarkPaid(ctx context.Context, id uuid.UUID, fromStatuses []payment.Status) error {
	db := database.DB(ctx, r.pool)
	var returnedID uuid.UUID
	err := db.QueryRow(ctx,
		`UPDATE payments SET status = 'success', paid_at = NOW() WHERE id = $1 AND status = ANY($2) RETURNING id`,
		id, fromStatuses,
	).Scan(&returnedID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return apperror.ErrConflict
		}
		return fmt.Errorf("marking payment paid: %w", err)
	}
	return nil
}

func scanPaymentByOrder(row pgx.CollectableRow) (payment.Payment, error) {
	var p payment.Payment
	var amt amountColumns
	var paymentMethodID, paymentURL, gatewayTxnID *string
	if err := row.Scan(&p.ID, &p.OrderID, &amt.amount, &amt.currency, &p.Status, &p.Method,
		&paymentMethodID, &paymentURL, &gatewayTxnID, &p.PaidAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return payment.Payment{}, fmt.Errorf("scanning payment: %w", err)
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
	return p, nil
}

func (r *Repository) ListByOrderID(ctx context.Context, orderID uuid.UUID) ([]payment.Payment, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT id, order_id, amount, currency, status, method, payment_method_id, payment_url,
		        gateway_txn_id, paid_at, created_at, updated_at
		FROM payments WHERE order_id = $1 ORDER BY created_at DESC`, orderID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing payments by order: %w", err)
	}

	payments, err := pgx.CollectRows(rows, scanPaymentByOrder)
	if err != nil {
		return nil, fmt.Errorf("iterating payments by order: %w", err)
	}
	return payments, nil
}

func (r *Repository) ListAdmin(ctx context.Context, params payment.AdminListParams) ([]payment.Payment, int, error) {
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

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(
		`SELECT id, order_id, amount, currency, status, method, payment_method_id, gateway_txn_id, paid_at, created_at, updated_at
		FROM payments WHERE %s ORDER BY created_at DESC LIMIT $%d OFFSET $%d`,
		where, argIdx, argIdx+1,
	)
	args = append(args, params.PageSize, offset)

	rows, err := db.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("listing payments: %w", err)
	}

	payments, err := pgx.CollectRows(rows, scanPaymentAdmin)
	if err != nil {
		return nil, 0, fmt.Errorf("iterating payments: %w", err)
	}
	return payments, total, nil
}

func scanPaymentAdmin(row pgx.CollectableRow) (payment.Payment, error) {
	var p payment.Payment
	var amt amountColumns
	var paymentMethodID, gatewayTxnID *string
	if err := row.Scan(&p.ID, &p.OrderID, &amt.amount, &amt.currency, &p.Status, &p.Method,
		&paymentMethodID, &gatewayTxnID, &p.PaidAt, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return payment.Payment{}, fmt.Errorf("scanning payment: %w", err)
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

// defaultJobMaxAttempts mirrors the payment_jobs.max_attempts column default.
const defaultJobMaxAttempts = 3

func (r *Repository) CreateJob(ctx context.Context, job *payment.Job) error {
	db := database.DB(ctx, r.pool)

	// Claim requires attempts < max_attempts, so a zero budget would strand the
	// job. Written back so the caller sees the budget it actually got.
	if job.MaxAttempts <= 0 {
		job.MaxAttempts = defaultJobMaxAttempts
	}

	err := db.QueryRow(ctx,
		`INSERT INTO payment_jobs (payment_id, order_id, action, status, max_attempts, locked_until, next_retry_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at, updated_at`,
		job.PaymentID, job.OrderID, job.Action, job.Status, job.MaxAttempts,
		job.LockedUntil, job.NextRetryAt,
	).Scan(&job.ID, &job.CreatedAt, &job.UpdatedAt)
	if err != nil {
		return fmt.Errorf("creating payment job: %w", err)
	}
	return nil
}

func (r *Repository) Claim(ctx context.Context, batchSize int, leaseDuration time.Duration) ([]payment.Job, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`WITH picked AS (
			SELECT id
			FROM payment_jobs
			WHERE (
				(status = 'pending' AND next_retry_at <= NOW())
				OR (status = 'processing' AND locked_until <= NOW())
			)
			AND attempts < max_attempts
			ORDER BY next_retry_at
			LIMIT $1
			FOR UPDATE SKIP LOCKED
		)
		UPDATE payment_jobs j
		SET status = 'processing',
		    locked_until = NOW() + $2::interval
		FROM picked
		WHERE j.id = picked.id
		RETURNING j.id, j.payment_id, j.order_id, j.action, j.status, j.attempts,
		          j.max_attempts, j.last_error, j.locked_until, j.next_retry_at,
		          j.created_at, j.updated_at`,
		batchSize, leaseDuration.String(),
	)
	if err != nil {
		return nil, fmt.Errorf("claiming pending jobs: %w", err)
	}

	jobs, err := pgx.CollectRows(rows, scanJob)
	if err != nil {
		return nil, fmt.Errorf("iterating payment jobs: %w", err)
	}
	return jobs, nil
}

func scanJob(row pgx.CollectableRow) (payment.Job, error) {
	var j payment.Job
	var lastError *string
	if err := row.Scan(&j.ID, &j.PaymentID, &j.OrderID, &j.Action, &j.Status,
		&j.Attempts, &j.MaxAttempts, &lastError, &j.LockedUntil, &j.NextRetryAt,
		&j.CreatedAt, &j.UpdatedAt); err != nil {
		return payment.Job{}, fmt.Errorf("scanning payment job: %w", err)
	}
	if lastError != nil {
		j.LastError = *lastError
	}
	return j, nil
}

func (r *Repository) UpdateJob(ctx context.Context, job *payment.Job) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payment_jobs SET status = $1, attempts = $2, last_error = $3,
		 locked_until = $4, next_retry_at = $5
		 WHERE id = $6`,
		job.Status, job.Attempts, nilIfEmpty(job.LastError),
		job.LockedUntil, job.NextRetryAt, job.ID,
	)
	if err != nil {
		return fmt.Errorf("updating payment job: %w", err)
	}
	return nil
}

func (r *Repository) CancelJobsByOrderID(ctx context.Context, orderID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payment_jobs SET status = 'cancelled' WHERE order_id = $1 AND status IN ('pending', 'processing')`,
		orderID,
	)
	if err != nil {
		return fmt.Errorf("cancelling jobs by order: %w", err)
	}
	return nil
}

func (r *Repository) MarkJobCompleted(ctx context.Context, jobID uuid.UUID) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payment_jobs SET status = 'completed', locked_until = NULL WHERE id = $1`,
		jobID,
	)
	if err != nil {
		return fmt.Errorf("marking job completed: %w", err)
	}
	return nil
}

func (r *Repository) MarkJobCompletedByPaymentID(ctx context.Context, paymentID uuid.UUID, action payment.JobAction) error {
	db := database.DB(ctx, r.pool)
	_, err := db.Exec(ctx,
		`UPDATE payment_jobs SET status = 'completed', locked_until = NULL
		 WHERE payment_id = $1 AND action = $2 AND status IN ('pending', 'processing')`,
		paymentID, action,
	)
	if err != nil {
		return fmt.Errorf("marking job completed by payment: %w", err)
	}
	return nil
}

func (r *Repository) Prune(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	db := database.DB(ctx, r.pool)
	tag, err := db.Exec(ctx,
		`DELETE FROM payment_jobs WHERE id IN (
			SELECT id FROM payment_jobs
			WHERE status IN ('completed', 'failed', 'cancelled')
			AND updated_at < NOW() - $1::interval
			LIMIT $2
		)`,
		olderThan.String(), limit,
	)
	if err != nil {
		return 0, fmt.Errorf("deleting old completed jobs: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

func nilIfEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
