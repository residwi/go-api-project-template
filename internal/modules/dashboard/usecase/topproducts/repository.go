package topproducts

import (
	"context"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
)

type Repository interface {
	ListTopProducts(ctx context.Context, limit int, from, to time.Time) ([]domain.TopProduct, error)
}
