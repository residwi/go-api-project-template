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
	ID        uuid.UUID
	UserID    uuid.UUID
	Type      Type
	Title     string
	Body      string
	IsRead    bool
	Data      []byte
	CreatedAt time.Time
}

type JobStatus string

const (
	JobStatusPending    JobStatus = "pending"
	JobStatusProcessing JobStatus = "processing"
	JobStatusCompleted  JobStatus = "completed"
	JobStatusFailed     JobStatus = "failed"
)

type Job struct {
	ID          uuid.UUID
	UserID      uuid.UUID
	Type        string
	Title       string
	Body        string
	Data        []byte
	Status      JobStatus
	Attempts    int
	MaxAttempts int
	LastError   string
	CreatedAt   time.Time
}
