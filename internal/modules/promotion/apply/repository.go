package apply

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

// Repository is apply's own storage. Its only implementation is
// apply/postgres, constructed in promotion/module.go.
type Repository interface {
	GetByCode(ctx context.Context, code string) (*domain.Promotion, error)
}
