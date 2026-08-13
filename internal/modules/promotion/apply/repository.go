package apply

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type Repository interface {
	GetByCode(ctx context.Context, code string) (*domain.Promotion, error)
}
