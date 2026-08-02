package payment

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
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

type JobAction string

const (
	ActionCharge JobAction = "charge"
	ActionRefund JobAction = "refund"
)

type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
	JobStatusCancelled  JobStatus = "cancelled"
)

type Payment struct {
	ID      uuid.UUID
	OrderID uuid.UUID
	// Amount is what this payment charges, denominated. Finalization verifies it
	// against the order's total with money.Money.Equal, which compares currency as
	// well as amount -- the pairing is what makes that single comparison equivalent
	// to the two-field check it replaced.
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

type Job struct {
	ID          uuid.UUID
	PaymentID   uuid.UUID
	OrderID     uuid.UUID
	Action      JobAction
	Status      JobStatus
	Attempts    int
	MaxAttempts int
	LastError   string
	LockedUntil *time.Time
	NextRetryAt time.Time
	CreatedAt   time.Time
	UpdatedAt   time.Time
}
