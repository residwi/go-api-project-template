package webhook

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

// Repository is webhook's own storage: only what resolving and reacting to a
// gateway callback needs.
type Repository interface {
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	GetByGatewayTxnID(ctx context.Context, txnID string) (*domain.Payment, error)
	UpdateStatus(ctx context.Context, id uuid.UUID, toStatus domain.Status, fromStatuses []domain.Status) error
	ClearPaymentURL(ctx context.Context, id uuid.UUID) error
}
