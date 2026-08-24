package domain

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/money"
)

type Status string

const (
	StatusPending        Status = "pending"
	StatusProcessing     Status = "processing"
	StatusSuccess        Status = "success"
	StatusFailed         Status = "failed"
	StatusCancelled      Status = "cancelled"
	StatusRequiresReview Status = "requires_review"
	StatusRefunded       Status = "refunded"
)

type Payment struct {
	ID              uuid.UUID
	OrderID         uuid.UUID
	Amount          money.Money
	Status          Status
	Method          string
	PaymentMethodID string
	PaymentURL      string
	GatewayTxnID    string
	GatewayResponse []byte
	PaidAt          *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
}
