package query

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
)

type Repository interface {
	ListAdmin(ctx context.Context, params Params) ([]domain.Promotion, int, error)
}
