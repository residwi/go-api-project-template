package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Payment, int, error)
}

type AdminListParams struct {
	paging.OffsetPage

	Status  string
	OrderID string
}
