package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/summary"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ summary.Repository = (*Repository)(nil)

func scanStatusBreakdown(row pgx.CollectableRow) (domain.StatusBreakdown, error) {
	var b domain.StatusBreakdown
	err := row.Scan(&b.Status, &b.Count)
	return b, err
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetSalesSummary(ctx context.Context, from, to time.Time) (domain.SalesSummary, error) {
	db := database.DB(ctx, r.pool)
	var s domain.SalesSummary
	err := db.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(total_amount), 0), COALESCE(AVG(total_amount), 0)
		FROM orders
		WHERE status IN ('paid', 'delivered', 'shipped') AND created_at BETWEEN $1 AND $2`,
		from, to,
	).Scan(&s.TotalOrders, &s.TotalRevenue, &s.AverageOrderValue)
	if err != nil {
		return domain.SalesSummary{}, fmt.Errorf("getting sales summary: %w", err)
	}
	return s, nil
}

func (r *Repository) GetOrderStatusBreakdown(
	ctx context.Context,
	from, to time.Time,
) ([]domain.StatusBreakdown, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT status, COUNT(*) FROM orders
		WHERE created_at BETWEEN $1 AND $2
		GROUP BY status ORDER BY COUNT(*) DESC`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("getting order status breakdown: %w", err)
	}
	breakdowns, err := pgx.CollectRows(rows, scanStatusBreakdown)
	if err != nil {
		return nil, fmt.Errorf("getting order status breakdown: %w", err)
	}

	return breakdowns, nil
}
