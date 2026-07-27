package dashboard

import (
	"context"
	"time"
)

// Repository is dashboard's persistence port. The Postgres implementation
// lives in the postgres subpackage; this package never imports it.
type Repository interface {
	GetSalesSummary(ctx context.Context, from, to time.Time) (SalesSummary, error)
	GetTopProducts(ctx context.Context, limit int, from, to time.Time) ([]TopProduct, error)
	GetRevenueByDay(ctx context.Context, from, to time.Time) ([]RevenueData, error)
	GetOrderStatusBreakdown(ctx context.Context, from, to time.Time) ([]StatusBreakdown, error)
}
