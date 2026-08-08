package jobs

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/modules/notification/domain"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestWorker_EnqueueOrderPlaced(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		w := New(repo, testhelper.DiscardLogger())

		ctx := context.Background()
		userID := uuid.New()
		orderID := uuid.New()

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *domain.Job) bool {
			return job.UserID == userID &&
				job.Type == string(domain.TypeOrderPlaced) &&
				job.Title == "Order Placed" &&
				job.Body == fmt.Sprintf("Your order %s has been placed.", orderID.String()) &&
				job.Status == domain.JobStatusPending &&
				job.Attempts == 0 &&
				job.MaxAttempts == 3
		})).Return(nil)

		err := w.EnqueueOrderPlaced(ctx, userID, orderID)
		require.NoError(t, err)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		w := New(repo, testhelper.DiscardLogger())

		ctx := context.Background()
		userID := uuid.New()
		orderID := uuid.New()

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *domain.Job) bool {
			return job.UserID == userID &&
				job.Body == fmt.Sprintf("Your order %s has been placed.", orderID.String())
		})).Return(assert.AnError)

		err := w.EnqueueOrderPlaced(ctx, userID, orderID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestWorker_Process(t *testing.T) {
	t.Parallel()

	t.Run("success", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		w := New(repo, testhelper.DiscardLogger())

		ctx := context.Background()
		userID := uuid.New()
		job := domain.Job{
			ID:     uuid.New(),
			UserID: userID,
			Type:   string(domain.TypeOrderPlaced),
			Title:  "Order Placed",
			Body:   "Your order has been placed.",
			Data:   []byte(`{"order_id":"abc"}`),
		}

		// Insert + completion commit atomically, so the job can't be re-claimed into a duplicate.
		repo.EXPECT().CreateAndComplete(mock.Anything,
			mock.MatchedBy(func(n *domain.Notification) bool {
				return n.UserID == userID &&
					n.Type == domain.TypeOrderPlaced &&
					n.Title == "Order Placed" &&
					n.Body == "Your order has been placed." &&
					n.IsRead == false &&
					string(n.Data) == `{"order_id":"abc"}`
			}),
			mock.MatchedBy(func(j *domain.Job) bool {
				return j.Status == domain.JobStatusCompleted
			}),
		).Return(nil)

		err := w.Process(ctx, job)
		require.NoError(t, err)
	})

	t.Run("repo error", func(t *testing.T) {
		t.Parallel()

		repo := NewMockRepository(t)
		w := New(repo, testhelper.DiscardLogger())

		ctx := context.Background()
		job := domain.Job{
			ID:          uuid.New(),
			UserID:      uuid.New(),
			Type:        string(domain.TypeOrderPlaced),
			Title:       "Order Placed",
			Body:        "Your order has been placed.",
			MaxAttempts: 3,
		}

		repo.EXPECT().CreateAndComplete(mock.Anything,
			mock.AnythingOfType("*domain.Notification"),
			mock.AnythingOfType("*domain.Job"),
		).Return(assert.AnError)
		// On failure the attempt is recorded so the job can reach a terminal state
		// instead of being re-claimed forever; here attempts<max, so it's requeued.
		repo.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.Attempts == 1 && j.Status == domain.JobStatusPending && j.LastError != ""
		})).Return(nil)

		err := w.Process(ctx, job)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
