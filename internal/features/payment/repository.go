package payment

import (
	"context"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/payment/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
)

type Repository interface {
	Create(ctx context.Context, p *domain.Payment) error
	GetActiveByOrderID(ctx context.Context, orderID uuid.UUID) (*domain.Payment, error)
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Payment, error)
	GetByGatewayTxnID(ctx context.Context, txnID string) (*domain.Payment, error)
	UpdateGateway(ctx context.Context, id uuid.UUID, txnID string, response []byte) error
	UpdatePaymentURL(ctx context.Context, id uuid.UUID, paymentURL string) error
	ClearPaymentURL(ctx context.Context, id uuid.UUID) error
	MarkPaid(ctx context.Context, id uuid.UUID, fromStatuses []domain.Status) error
	UpdateStatus(ctx context.Context, id uuid.UUID, toStatus domain.Status, fromStatuses []domain.Status) error
	ListAdmin(ctx context.Context, params AdminListParams) ([]domain.Payment, int, error)
}

type AdminListParams struct {
	paging.OffsetPage

	Status  string
	OrderID string
}
