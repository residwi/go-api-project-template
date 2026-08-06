// Package contract is payment's published surface. It imports no module and no
// platform package.
package contract

import (
	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
)

type ChargeRequest struct {
	OrderID         uuid.UUID
	Amount          money.Money
	PaymentMethodID string
}

// ChargeResult reports what the gateway did. Charged is false when the gateway
// handed back a URL for the customer to complete payment instead of charging
// inline.
type ChargeResult struct {
	PaymentID  uuid.UUID
	PaymentURL string
	Charged    bool
}
