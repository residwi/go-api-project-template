package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

// Repository is query's own storage: the admin-facing reads only. Only its
// implementation is query/postgres, constructed in payment/module.go.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Payment, int, error)
}

type AdminListParams struct {
	paging.OffsetPage

	Status  string
	OrderID string
}
