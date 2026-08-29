package jobs

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/residwi/go-api-project-template/internal/platform/database"
	"github.com/residwi/go-api-project-template/internal/platform/queue"
)

type Queue struct {
	client *river.Client[pgx.Tx]
	db     database.DB
}

func NewQueue(client *river.Client[pgx.Tx], db database.DB) *Queue {
	return &Queue{client: client, db: db}
}

func (q *Queue) EnqueueSend(ctx context.Context, notificationID uuid.UUID) error {
	return queue.Insert(ctx, q.client, q.db, SendArgs{NotificationID: notificationID}, nil)
}
