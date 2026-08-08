package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/modules/dashboard/topproducts"
	"github.com/residwi/go-api-project-template/internal/platform/database"
)

var _ topproducts.Repository = (*Repository)(nil)

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

func (r *Repository) GetTopProducts(
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
