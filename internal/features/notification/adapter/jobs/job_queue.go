package jobs

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/jobqueue"
)

type JobQueue struct {
	client *river.Client[pgx.Tx]
	db     database.DB
}

func NewJobQueue(client *river.Client[pgx.Tx], db database.DB) *JobQueue {
	return &JobQueue{client: client, db: db}
}

func (q *JobQueue) EnqueueSend(ctx context.Context, notificationID uuid.UUID) error {
	return jobqueue.Insert(ctx, q.client, q.db, SendArgs{NotificationID: notificationID}, nil)
}
