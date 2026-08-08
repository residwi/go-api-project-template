package create

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

// Repository is create's own storage. Its only implementation is
// create/postgres, constructed in promotion/module.go.
type Repository interface {
	Create(ctx context.Context, promo *domain.Promotion) error
}
