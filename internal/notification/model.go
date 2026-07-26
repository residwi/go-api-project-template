package notification

import (
	"time"

	"github.com/google/uuid"
)

type Type string

const (
	TypeOrderPlaced    Type = "order_placed"
	TypeOrderShipped   Type = "order_shipped"
	TypePaymentSuccess Type = "payment_success"
	TypePaymentFailed  Type = "payment_failed"
)

type Notification struct {
	ID        uuid.UUID `json:"id"`
	UserID    uuid.UUID `json:"user_id"`
	Type      Type      `json:"type"`
	Title     string    `json:"title"`
	Body      string    `json:"body,omitempty"`
	IsRead    bool      `json:"is_read"`
	Data      []byte    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

type Job struct {
	ID          uuid.UUID `json:"id"`
	UserID      uuid.UUID `json:"user_id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Body        string    `json:"body"`
	Data        []byte    `json:"-"`
	Status      JobStatus `json:"status"`
	Attempts    int       `json:"attempts"`
	MaxAttempts int       `json:"max_attempts"`
	LastError   string    `json:"last_error,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}
