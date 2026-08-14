package summary

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

type Repository interface {
	GetSalesSummary(ctx context.Context, from, to time.Time) (domain.SalesSummary, error)
	ListOrderStatusBreakdown(ctx context.Context, from, to time.Time) ([]domain.StatusBreakdown, error)
}
