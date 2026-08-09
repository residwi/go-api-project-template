package refund

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/apperror"
	inventorycontract "github.com/residwi/go-api-project-template/internal/modules/inventory/contract"
	ordercontract "github.com/residwi/go-api-project-template/internal/modules/order/contract"
	"github.com/residwi/go-api-project-template/internal/modules/payment/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment/gateway"
	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/testhelper"
)

func TestCommand_Execute(t *testing.T) {
	t.Parallel()

	ctx := context.Background()

	t.Run("success enqueues a pending refund job from a succeeded payment", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, jobs, _ := newTestCommand(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		jobs.EXPECT().EnqueueRefund(mock.Anything, paymentID, orderID).
			Return(nil)

		err := cmd.Execute(ctx, paymentID)

		require.NoError(t, err)
	})

	t.Run("success enqueues a pending refund job from a requires-review payment", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, jobs, _ := newTestCommand(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusRequiresReview,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		jobs.EXPECT().EnqueueRefund(mock.Anything, paymentID, orderID).
			Return(nil)

		err := cmd.Execute(ctx, paymentID)

		require.NoError(t, err)
	})

	t.Run("payment not refundable - wrong status", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, _, _ := newTestCommand(t)

		paymentID := uuid.New()

		p := &domain.Payment{
			ID:     paymentID,
			Status: domain.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		err := cmd.Execute(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("payment not refundable - cancelled status", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, _, _ := newTestCommand(t)

		paymentID := uuid.New()

		p := &domain.Payment{
			ID:     paymentID,
			Status: domain.StatusCancelled,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		err := cmd.Execute(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrBadRequest)
	})

	t.Run("payment not found", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, _, _ := newTestCommand(t)

		paymentID := uuid.New()

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(nil, apperror.ErrNotFound)

		err := cmd.Execute(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorIs(t, err, apperror.ErrNotFound)
	})

	t.Run("enqueue error", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, jobs, _ := newTestCommand(t)

		paymentID := uuid.New()
		orderID := uuid.New()

		p := &domain.Payment{
			ID:      paymentID,
			OrderID: orderID,
			Status:  domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, paymentID).
			Return(p, nil)

		jobs.EXPECT().EnqueueRefund(mock.Anything, paymentID, orderID).
			Return(errors.New("insert job failed"))

		err := cmd.Execute(ctx, paymentID)

		require.Error(t, err)
		assert.ErrorContains(t, err, "insert job failed")
	})
}

func TestCommand_ProcessRefund(t *testing.T) {
	t.Parallel()

	t.Run("success with release inventory", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, gw, orders, orderGet, orderItems, inventoryRestore, jobs, couponRel := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_123",
			Status:       domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, gateway.RefundRequest{
			IdempotencyKey: job.PaymentID.String(),
			TransactionID:  "txn_123",
			Amount:         5000,
			Reason:         "auto-refund",
		}).Return(gateway.RefundResponse{RefundID: "ref_001"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 2}, nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{CouponCode: "SAVE10", StockDeducted: false}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventorycontract.Reserved).
			Return(nil)

		couponRel.EXPECT().Release(mock.Anything, job.OrderID).
			Return(nil)

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := cmd.ProcessRefund(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("success with restock inventory", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, gw, orders, orderGet, orderItems, inventoryRestore, jobs, _ := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(8000, "USD"),
			GatewayTxnID: "txn_456",
			Status:       domain.StatusRequiresReview,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, gateway.RefundRequest{
			IdempotencyKey: job.PaymentID.String(),
			TransactionID:  "txn_456",
			Amount:         8000,
			Reason:         "auto-refund",
		}).Return(gateway.RefundResponse{RefundID: "ref_002"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 5}, nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{StockDeducted: true}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventorycontract.Deducted).
			Return(nil)

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := cmd.ProcessRefund(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("payment not refundable", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, jobs, _ := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    domain.ActionRefund,
		}

		p := &domain.Payment{
			ID:     job.PaymentID,
			Status: domain.StatusPending,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID && j.Status == domain.JobStatusCancelled
		})).Return(nil)

		processErr := cmd.ProcessRefund(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("payment not found", func(t *testing.T) {
		t.Parallel()

		cmd, repo, _, _, _, _, _, _, _ := newTestCommand(t)

		job := domain.Job{
			ID:        uuid.New(),
			PaymentID: uuid.New(),
			OrderID:   uuid.New(),
			Action:    domain.ActionRefund,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(nil, errors.New("not found"))

		processErr := cmd.ProcessRefund(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("gateway refund error with retries remaining", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, _, _, _, _, jobs, _ := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_789",
			Status:       domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{}, errors.New("gateway timeout"))

		jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusPending &&
				j.Attempts == 1 &&
				j.LastError == "gateway timeout"
		})).Return(nil)

		processErr := cmd.ProcessRefund(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("gateway refund error max attempts", func(t *testing.T) {
		t.Parallel()

		cmd, repo, gw, _, _, _, _, jobs, _ := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    2,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_999",
			Status:       domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{}, errors.New("gateway error"))

		jobs.EXPECT().UpdateJob(mock.Anything, mock.MatchedBy(func(j *domain.Job) bool {
			return j.ID == job.ID &&
				j.Status == domain.JobStatusFailed &&
				j.Attempts == 3
		})).Return(nil)

		processErr := cmd.ProcessRefund(context.Background(), job)
		assert.Error(t, processErr)
	})

	t.Run("refund with list items error returns false", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, gw, orders, orderGet, orderItems, _, _, _ := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_items_err",
			Status:       domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_items_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{}, nil)

		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(nil, errors.New("db error"))

		processErr := cmd.ProcessRefund(ctx, job)
		assert.Error(t, processErr)
	})

	t.Run("refund restores every product the map holds", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, gw, orders, orderGet, orderItems, inventoryRestore, jobs, _ := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_multi",
			Status:       domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_multi"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID1 := uuid.New()
		productID2 := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID2: 1, productID1: 2}, nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{StockDeducted: false}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventorycontract.Reserved).
			Return(nil)

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := cmd.ProcessRefund(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("refund with release inventory error logs but continues", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, gw, orders, orderGet, orderItems, inventoryRestore, jobs, _ := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_rel_err",
			Status:       domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_rel_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{StockDeducted: false}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventorycontract.Reserved).
			Return(errors.New("release failed"))

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := cmd.ProcessRefund(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("refund with restock inventory error logs but continues", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, gw, orders, orderGet, orderItems, inventoryRestore, jobs, _ := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_restock_err",
			Status:       domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_restock_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{StockDeducted: true}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventorycontract.Deducted).
			Return(errors.New("restock failed"))

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := cmd.ProcessRefund(ctx, job)
		assert.NoError(t, processErr)
	})

	t.Run("refund with coupon release error logs but continues", func(t *testing.T) {
		t.Parallel()

		ctx := context.Background()
		cmd, repo, gw, orders, orderGet, orderItems, inventoryRestore, jobs, couponRel := newTestCommand(t)

		job := domain.Job{
			ID:          uuid.New(),
			PaymentID:   uuid.New(),
			OrderID:     uuid.New(),
			Action:      domain.ActionRefund,
			Attempts:    0,
			MaxAttempts: 3,
		}

		p := &domain.Payment{
			ID:           job.PaymentID,
			OrderID:      job.OrderID,
			Amount:       money.New(5000, "USD"),
			GatewayTxnID: "txn_coupon_err",
			Status:       domain.StatusSuccess,
		}

		repo.EXPECT().GetByID(mock.Anything, job.PaymentID).
			Return(p, nil)

		gw.EXPECT().Refund(mock.Anything, mock.Anything).
			Return(gateway.RefundResponse{RefundID: "ref_coupon_err"}, nil)

		repo.EXPECT().UpdateStatus(mock.Anything, job.PaymentID, domain.StatusRefunded,
			[]domain.Status{domain.StatusSuccess, domain.StatusRequiresReview}).
			Return(nil)

		orders.EXPECT().MarkRefunded(mock.Anything, job.OrderID).
			Return(nil)

		productID := uuid.New()
		orderItems.EXPECT().ListItemQuantities(mock.Anything, job.OrderID).
			Return(map[uuid.UUID]int{productID: 1}, nil)

		orderGet.EXPECT().GetSnapshot(mock.Anything, job.OrderID).
			Return(ordercontract.Order{CouponCode: "SAVE10", StockDeducted: false}, nil)

		inventoryRestore.EXPECT().Restore(mock.Anything, mock.Anything, inventorycontract.Reserved).
			Return(nil)

		couponRel.EXPECT().Release(mock.Anything, job.OrderID).
			Return(errors.New("coupon release failed"))

		jobs.EXPECT().MarkJobCompleted(mock.Anything, job.ID).
			Return(nil)

		processErr := cmd.ProcessRefund(ctx, job)
		assert.NoError(t, processErr)
	})
}

func newTestCommand(t *testing.T) (
	*Command,
	*MockRepository,
	*MockGateway,
	*MockOrderUpdater,
	*MockOrderGetter,
	*MockOrderItemsGetter,
	*MockInventoryRestorer,
	*MockJobStore,
	*MockCouponReleaser,
) {
	repo := NewMockRepository(t)
	gw := NewMockGateway(t)
	orders := NewMockOrderUpdater(t)
	orderGet := NewMockOrderGetter(t)
	orderItems := NewMockOrderItemsGetter(t)
	inventoryRestore := NewMockInventoryRestorer(t)
	jobs := NewMockJobStore(t)
	couponRel := NewMockCouponReleaser(t)

	cmd := New(
		repo, testhelper.FakeTxRunner{}, gw, orders, orderGet, orderItems,
		inventoryRestore, couponRel, jobs,
		testhelper.DiscardLogger(),
	)

	return cmd, repo, gw, orders, orderGet, orderItems, inventoryRestore, jobs, couponRel
}
