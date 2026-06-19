package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

func scanTopProduct(row pgx.CollectableRow) (TopProduct, error) {
	var p TopProduct
	err := row.Scan(&p.ProductID, &p.Name, &p.TotalSold, &p.Revenue)
	return p, err
}

func scanRevenueData(row pgx.CollectableRow) (RevenueData, error) {
	var d RevenueData
	err := row.Scan(&d.Date, &d.Revenue, &d.OrderCount)
	return d, err
}

func scanStatusBreakdown(row pgx.CollectableRow) (StatusBreakdown, error) {
	var b StatusBreakdown
	err := row.Scan(&b.Status, &b.Count)
	return b, err
}

type Repository interface {
	GetSalesSummary(ctx context.Context, from, to time.Time) (SalesSummary, error)
	GetTopProducts(ctx context.Context, limit int, from, to time.Time) ([]TopProduct, error)
	GetRevenueByDay(ctx context.Context, from, to time.Time) ([]RevenueData, error)
	GetOrderStatusBreakdown(ctx context.Context, from, to time.Time) ([]StatusBreakdown, error)
}

type PostgresRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresRepository(pool *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{pool: pool}
}

func (r *PostgresRepository) GetSalesSummary(ctx context.Context, from, to time.Time) (SalesSummary, error) {
	db := database.DB(ctx, r.pool)
	var s SalesSummary
	err := db.QueryRow(ctx,
		`SELECT COUNT(*), COALESCE(SUM(total_amount), 0), COALESCE(AVG(total_amount), 0)
		FROM orders
		WHERE status IN ('paid', 'delivered', 'shipped') AND created_at BETWEEN $1 AND $2`,
		from, to,
	).Scan(&s.TotalOrders, &s.TotalRevenue, &s.AverageOrderValue)
	if err != nil {
		return SalesSummary{}, fmt.Errorf("getting sales summary: %w", err)
	}
	return s, nil
}

func (r *PostgresRepository) GetTopProducts(ctx context.Context, limit int, from, to time.Time) ([]TopProduct, error) {
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

func (r *PostgresRepository) GetRevenueByDay(ctx context.Context, from, to time.Time) ([]RevenueData, error) {
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

func (r *PostgresRepository) GetOrderStatusBreakdown(ctx context.Context, from, to time.Time) ([]StatusBreakdown, error) {
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

var _ Repository = (*PostgresRepository)(nil)
