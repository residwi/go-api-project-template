package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/revenue"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ revenue.Repository = (*Repository)(nil)

func scanRevenueData(row pgx.CollectableRow) (domain.RevenueData, error) {
	var d domain.RevenueData
	err := row.Scan(&d.Date, &d.Revenue, &d.OrderCount)
	return d, err
}

type Repository struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) GetRevenueByDay(ctx context.Context, from, to time.Time) ([]domain.RevenueData, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT DATE(created_at) AS date, COALESCE(SUM(total_amount), 0) AS revenue, COUNT(*) AS order_count
		FROM orders
		WHERE status IN ('paid', 'delivered', 'shipped') AND created_at BETWEEN $1 AND $2
		GROUP BY DATE(created_at)
		ORDER BY date`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("getting revenue by day: %w", err)
	}
	data, err := pgx.CollectRows(rows, scanRevenueData)
	if err != nil {
		return nil, fmt.Errorf("getting revenue by day: %w", err)
	}

	return data, nil
}
