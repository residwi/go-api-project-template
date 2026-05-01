package payment

import (
	"time"

	"github.com/google/uuid"
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
	ID              uuid.UUID  `json:"id"`
	OrderID         uuid.UUID  `json:"order_id"`
	Amount          int64      `json:"amount"`
	Currency        string     `json:"currency"`
	Status          Status     `json:"status"`
	Method          string     `json:"method,omitempty"`
	PaymentMethodID string     `json:"payment_method_id,omitempty"`
	PaymentURL      string     `json:"payment_url,omitempty"`
	GatewayTxnID    string     `json:"gateway_txn_id,omitempty"`
	GatewayResponse []byte     `json:"-"`
	PaidAt          *time.Time `json:"paid_at,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Job struct {
	ID              uuid.UUID  `json:"id"`
	PaymentID       uuid.UUID  `json:"payment_id"`
	OrderID         uuid.UUID  `json:"order_id"`
	Action          JobAction  `json:"action"`
	Status          JobStatus  `json:"status"`
	Attempts        int        `json:"attempts"`
	MaxAttempts     int        `json:"max_attempts"`
	LastError       string     `json:"last_error,omitempty"`
	LockedUntil     *time.Time `json:"locked_until,omitempty"`
	NextRetryAt     time.Time  `json:"next_retry_at"`
	InventoryAction string     `json:"inventory_action,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}
