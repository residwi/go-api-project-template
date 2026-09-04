package jobs

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

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

func (q *JobQueue) EnqueueRefund(ctx context.Context, paymentID, orderID uuid.UUID) error {
	args := RefundArgs{PaymentID: paymentID, OrderID: orderID}
	opts := &river.InsertOpts{Tags: []string{orderTag(orderID)}}

	return jobqueue.Insert(ctx, q.client, q.db, args, opts)
}

func (q *JobQueue) CancelPendingForOrder(ctx context.Context, orderID uuid.UUID) error {
	res, err := q.client.JobList(ctx, river.NewJobListParams().
		Kinds(RefundArgs{}.Kind()).
		TagsAny(orderTag(orderID)).
		States(
			rivertype.JobStateAvailable,
			rivertype.JobStateScheduled,
			rivertype.JobStateRetryable,
			rivertype.JobStatePending,
		))
	if err != nil {
		return fmt.Errorf("listing payment jobs for order %s: %w", orderID, err)
	}

	for _, job := range res.Jobs {
		if _, err := q.client.JobCancel(ctx, job.ID); err != nil {
			return fmt.Errorf("cancelling payment job %d: %w", job.ID, err)
		}
	}

	return nil
}

func orderTag(orderID uuid.UUID) string { return "order-" + orderID.String() }
