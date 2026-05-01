package dashboard

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/platform/database"
)

type Repository interface {
	GetSalesSummary(ctx context.Context, from, to time.Time) (SalesSummary, error)
	GetTopProducts(ctx context.Context, limit int, from, to time.Time) ([]TopProduct, error)
	GetRevenueByDay(ctx context.Context, from, to time.Time) ([]RevenueData, error)
	GetOrderStatusBreakdown(ctx context.Context) ([]StatusBreakdown, error)
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
	defer rows.Close()

	var products []TopProduct
	for rows.Next() {
		var p TopProduct
		if err := rows.Scan(&p.ProductID, &p.Name, &p.TotalSold, &p.Revenue); err != nil {
			return nil, fmt.Errorf("scanning top product: %w", err)
		}
		products = append(products, p)
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
	defer rows.Close()

	var data []RevenueData
	for rows.Next() {
		var d RevenueData
		if err := rows.Scan(&d.Date, &d.Revenue, &d.OrderCount); err != nil {
			return nil, fmt.Errorf("scanning revenue data: %w", err)
		}
		data = append(data, d)
	}

	return data, nil
}

func (r *PostgresRepository) GetOrderStatusBreakdown(ctx context.Context) ([]StatusBreakdown, error) {
	db := database.DB(ctx, r.pool)
	rows, err := db.Query(ctx,
		`SELECT status, COUNT(*) FROM orders GROUP BY status ORDER BY count DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("getting order status breakdown: %w", err)
	}
	defer rows.Close()

	var breakdowns []StatusBreakdown
	for rows.Next() {
		var b StatusBreakdown
		if err := rows.Scan(&b.Status, &b.Count); err != nil {
			return nil, fmt.Errorf("scanning status breakdown: %w", err)
		}
		breakdowns = append(breakdowns, b)
	}

	return breakdowns, nil
}

var _ Repository = (*PostgresRepository)(nil)
