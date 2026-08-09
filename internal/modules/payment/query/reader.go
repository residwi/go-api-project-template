// Package query serves the two admin-facing reads payment publishes. Every
// other payment capability -- charging, refunding, the webhook and the
// worker queue -- lives in its own slice.
package query

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

type Reader struct {
	repo Repository
}

func New(repo Repository) *Reader {
	return &Reader{repo: repo}
}

func (r *Reader) GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error) {
	return r.repo.GetByID(ctx, id)
}

func (r *Reader) ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Payment, int, error) {
	return r.repo.ListAdmin(ctx, params)
}
