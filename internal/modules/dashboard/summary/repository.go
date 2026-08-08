package summary

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

// Repository is summary's own storage. Its only implementation is
// summary/postgres, constructed in dashboard/module.go.
type Repository interface {
	GetSalesSummary(ctx context.Context, from, to time.Time) (domain.SalesSummary, error)
	GetOrderStatusBreakdown(ctx context.Context, from, to time.Time) ([]domain.StatusBreakdown, error)
}
