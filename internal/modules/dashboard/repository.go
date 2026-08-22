package dashboard

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

type Repository interface {
	ListRevenueByDay(ctx context.Context, from, to time.Time) ([]domain.RevenueData, error)
	GetSalesSummary(ctx context.Context, from, to time.Time) (domain.SalesSummary, error)
	ListOrderStatusBreakdown(ctx context.Context, from, to time.Time) ([]domain.StatusBreakdown, error)
	ListTopProducts(ctx context.Context, limit int, from, to time.Time) ([]domain.TopProduct, error)
}
