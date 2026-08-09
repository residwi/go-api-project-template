package refund

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

// Repository is refund's own storage: checking refundability and flipping the
// payment to refunded. Everything else on the payments table belongs to
// whichever slice actually calls it -- charge, webhook, query.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, toStatus domain.Status, fromStatuses []domain.Status) error
}
