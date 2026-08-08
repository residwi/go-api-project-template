package topproducts

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

// Repository is topproducts' own storage. Its only implementation is
// topproducts/postgres, constructed in dashboard/module.go.
type Repository interface {
	ListTopProducts(ctx context.Context, limit int, from, to time.Time) ([]domain.TopProduct, error)
}
