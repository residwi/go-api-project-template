package revenue

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

// Repository is revenue's own storage. Its only implementation is
// revenue/postgres, constructed in dashboard/module.go.
type Repository interface {
	ListRevenueByDay(ctx context.Context, from, to time.Time) ([]domain.RevenueData, error)
}
