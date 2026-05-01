package notification_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/core"
	"github.com/residwi/go-api-project-template/internal/features/notification"
	mocks "github.com/residwi/go-api-project-template/mocks/notification"
)

func TestService_Send(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(n *notification.Notification) bool {
			return n.UserID == userID &&
				n.Type == notification.TypeOrderPlaced &&
				n.Title == "Order Confirmed" &&
				n.Body == "Your order has been confirmed." &&
				n.IsRead == false
		})).Return(nil)

		err := svc.Send(ctx, userID, notification.TypeOrderPlaced, "Order Confirmed", "Your order has been confirmed.", nil)
		require.NoError(t, err)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(assert.AnError)

		err := svc.Send(ctx, userID, notification.TypeOrderPlaced, "Title", "Body", nil)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_ListByUser(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		cursor := core.CursorPage{Limit: 20}
		expected := []notification.Notification{
			{ID: uuid.New(), UserID: userID, Type: notification.TypeOrderPlaced, Title: "Order Placed"},
			{ID: uuid.New(), UserID: userID, Type: notification.TypeOrderShipped, Title: "Order Shipped"},
		}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(expected, nil)

		result, err := svc.ListByUser(ctx, userID, cursor)
		require.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		cursor := core.CursorPage{Limit: 20}

		repo.EXPECT().ListByUser(mock.Anything, userID, cursor).Return(nil, assert.AnError)

		_, err := svc.ListByUser(ctx, userID, cursor)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_MarkRead(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		id := uuid.New()

		repo.EXPECT().MarkRead(mock.Anything, id).Return(nil)

		err := svc.MarkRead(ctx, id)
		require.NoError(t, err)
	})

	t.Run("not found", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		id := uuid.New()

		repo.EXPECT().MarkRead(mock.Anything, id).Return(core.ErrNotFound)

		err := svc.MarkRead(ctx, id)
		assert.ErrorIs(t, err, core.ErrNotFound)
	})
}

func TestService_MarkAllRead(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().MarkAllRead(mock.Anything, userID).Return(nil)

		err := svc.MarkAllRead(ctx, userID)
		require.NoError(t, err)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().MarkAllRead(mock.Anything, userID).Return(assert.AnError)

		err := svc.MarkAllRead(ctx, userID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_CountUnread(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().CountUnread(mock.Anything, userID).Return(5, nil)

		count, err := svc.CountUnread(ctx, userID)
		require.NoError(t, err)
		assert.Equal(t, 5, count)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()

		repo.EXPECT().CountUnread(mock.Anything, userID).Return(0, assert.AnError)

		_, err := svc.CountUnread(ctx, userID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_EnqueueOrderPlaced(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		orderID := uuid.New()

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *notification.Job) bool {
			return job.UserID == userID &&
				job.Type == string(notification.TypeOrderPlaced) &&
				job.Title == "Order Placed" &&
				job.Body == fmt.Sprintf("Your order %s has been placed.", orderID.String()) &&
				job.Status == "pending" &&
				job.Attempts == 0 &&
				job.MaxAttempts == 3
		})).Return(nil)

		err := svc.EnqueueOrderPlaced(ctx, userID, orderID)
		require.NoError(t, err)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		orderID := uuid.New()

		repo.EXPECT().CreateJob(mock.Anything, mock.MatchedBy(func(job *notification.Job) bool {
			return job.UserID == userID &&
				job.Body == fmt.Sprintf("Your order %s has been placed.", orderID.String())
		})).Return(assert.AnError)

		err := svc.EnqueueOrderPlaced(ctx, userID, orderID)
		assert.ErrorIs(t, err, assert.AnError)
	})
}

func TestService_ProcessJob(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		userID := uuid.New()
		job := notification.Job{
			ID:     uuid.New(),
			UserID: userID,
			Type:   string(notification.TypeOrderPlaced),
			Title:  "Order Placed",
			Body:   "Your order has been placed.",
			Data:   []byte(`{"order_id":"abc"}`),
		}

		repo.EXPECT().Create(mock.Anything, mock.MatchedBy(func(n *notification.Notification) bool {
			return n.UserID == userID &&
				n.Type == notification.TypeOrderPlaced &&
				n.Title == "Order Placed" &&
				n.Body == "Your order has been placed." &&
				n.IsRead == false &&
				string(n.Data) == `{"order_id":"abc"}`
		})).Return(nil)

		err := svc.ProcessJob(ctx, job)
		require.NoError(t, err)
	})

	t.Run("repo error", func(t *testing.T) {
		repo := mocks.NewMockRepository(t)
		svc := notification.NewService(repo)

		ctx := context.Background()
		job := notification.Job{
			ID:     uuid.New(),
			UserID: uuid.New(),
			Type:   string(notification.TypeOrderPlaced),
			Title:  "Order Placed",
			Body:   "Your order has been placed.",
		}

		repo.EXPECT().Create(mock.Anything, mock.AnythingOfType("*notification.Notification")).Return(assert.AnError)

		err := svc.ProcessJob(ctx, job)
		assert.ErrorIs(t, err, assert.AnError)
	})
}
