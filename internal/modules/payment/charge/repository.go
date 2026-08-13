package charge

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
)

type Repository interface {
	Create(ctx context.Context, p *domain.Payment) error
	GetActiveByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	UpdateGateway(ctx context.Context, id uuid.UUID, txnID string, response []byte) error
	UpdatePaymentURL(ctx context.Context, id uuid.UUID, paymentURL string) error
	MarkPaid(ctx context.Context, id uuid.UUID, fromStatuses []domain.Status) error
	UpdateStatus(ctx context.Context, id uuid.UUID, toStatus domain.Status, fromStatuses []domain.Status) error
}
