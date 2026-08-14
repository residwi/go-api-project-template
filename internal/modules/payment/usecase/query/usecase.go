package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

type UseCase struct {
	repo Repository
}

func New(repo Repository) *UseCase {
	return &UseCase{repo: repo}
}

func (r *UseCase) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	return r.repo.GetByID(ctx, id)
}

func (r *UseCase) ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Payment, int, error) {
	return r.repo.ListAdmin(ctx, params)
}
