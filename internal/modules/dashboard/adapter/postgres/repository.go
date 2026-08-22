package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ dashboard.Repository = (*Repository)(nil)

func scanStatusBreakdown(row pgx.CollectableRow) (domain.StatusBreakdown, error) {
	var b domain.StatusBreakdown
	err := row.Scan(&b.Status, &b.Count)
	return b, err
}

func scanRevenueData(row pgx.CollectableRow) (domain.RevenueData, error) {
	var d domain.RevenueData
	err := row.Scan(&d.Date, &d.Revenue, &d.OrderCount)
	return d, err
}

func scanTopProduct(row pgx.CollectableRow) (domain.TopProduct, error) {
	var p domain.TopProduct
	err := row.Scan(&p.ProductID, &p.Name, &p.TotalSold, &p.Revenue)
	return p, err
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

func (r *Repository) ListOrderStatusBreakdown(
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

func (r *Repository) ListRevenueByDay(ctx context.Context, from, to time.Time) ([]domain.RevenueData, error) {
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

func (r *Repository) ListTopProducts(
	ctx context.Context,
	limit int,
	from, to time.Time,
) ([]domain.TopProduct, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT oi.product_id, oi.product_name, SUM(oi.quantity) AS total_sold, SUM(oi.subtotal) AS revenue
		FROM order_items oi
		JOIN orders o ON o.id = oi.order_id
		WHERE o.status IN ('paid', 'delivered', 'shipped') AND o.created_at BETWEEN $1 AND $2
		GROUP BY oi.product_id, oi.product_name
		ORDER BY total_sold DESC
		LIMIT $3`,
		from, to, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("getting top products: %w", err)
	}
	products, err := pgx.CollectRows(rows, scanTopProduct)
	if err != nil {
		return nil, fmt.Errorf("getting top products: %w", err)
	}

	return products, nil
}
