package payment

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/money"
)

type ChargeRequest struct {
	OrderID         uuid.UUID
	Amount          money.Money
	PaymentMethodID string
}

type ChargeResult struct {
	PaymentID  uuid.UUID
	PaymentURL string
	Charged    bool
}
