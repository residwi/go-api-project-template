package payment_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/residwi/go-api-project-template/internal/features/payment"
	gateway "github.com/residwi/go-api-project-template/internal/platform/payment"
	mocks "github.com/residwi/go-api-project-template/mocks/payment"
)

// paymentMocks bundles the collaborators payment.Service depends on, so a test
// can set expectations on the ones it exercises without a long argument list.
type paymentMocks struct {
	orderUpdater     *mocks.MockOrderUpdater
	orderGet         *mocks.MockOrderGetter
	orderItems       *mocks.MockOrderItemsGetter
	inventoryDeduct  *mocks.MockInventoryDeductor
	inventoryRestore *mocks.MockInventoryRestorer
	couponRel        *mocks.MockCouponReleaser
	gw               *mocks.MockGateway
}

// newIntegrationService wires a real Postgres repository to mocked cross-feature
// collaborators. It sets no expectations — each test sets its own.
func newIntegrationService(t *testing.T) (*payment.Service, paymentMocks) {
	t.Helper()
	repo := payment.NewPostgresRepository(testPool)
	m := paymentMocks{
		orderUpdater:     mocks.NewMockOrderUpdater(t),
		orderGet:         mocks.NewMockOrderGetter(t),
		orderItems:       mocks.NewMockOrderItemsGetter(t),
		inventoryDeduct:  mocks.NewMockInventoryDeductor(t),
		inventoryRestore: mocks.NewMockInventoryRestorer(t),
		couponRel:        mocks.NewMockCouponReleaser(t),
		gw:               mocks.NewMockGateway(t),
	}
	svc := payment.NewService(repo, testPool, m.gw, m.orderUpdater, m.orderGet, m.orderItems,
		m.inventoryDeduct, m.inventoryRestore, m.couponRel)
	return svc, m
}

func TestService_ProcessIntegration(t *testing.T) {
	t.Run("processes a pending charge job to completion", func(t *testing.T) {
		ctx := context.Background()
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)

		repo := payment.NewPostgresRepository(testPool)
		job := &payment.Job{
			PaymentID:   p.ID,
			OrderID:     orderID,
			Action:      payment.ActionCharge,
			Status:      payment.JobStatusPending,
			MaxAttempts: 3,
			NextRetryAt: time.Now(),
		}
		require.NoError(t, repo.CreateJob(ctx, job))
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		svc, m := newIntegrationService(t)
		m.orderUpdater.EXPECT().MarkPaymentProcessing(mock.Anything, orderID).Return(nil)
		m.orderUpdater.EXPECT().MarkPaid(mock.Anything, orderID).Return(nil)
		m.orderGet.EXPECT().GetByID(mock.Anything, orderID).
			Return(payment.OrderSnapshot{TotalAmount: 1000, Currency: "USD", Status: "awaiting_payment"}, nil)
		m.orderItems.EXPECT().ListItemsByOrderID(mock.Anything, orderID).
			Return([]payment.OrderItemDTO{{ProductID: uuid.New(), Quantity: 1}}, nil)
		m.inventoryDeduct.EXPECT().DeductBatch(mock.Anything, mock.Anything).Return(nil)
		m.gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{TransactionID: "txn-" + uuid.New().String(), Status: "success"}, nil)

		require.NoError(t, svc.Process(ctx, *job))

		var jobStatus, paymentStatus string
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT status FROM payment_jobs WHERE id = $1`, job.ID).Scan(&jobStatus))
		assert.Equal(t, string(payment.JobStatusCompleted), jobStatus)
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT status FROM payments WHERE id = $1`, p.ID).Scan(&paymentStatus))
		assert.Equal(t, string(payment.StatusSuccess), paymentStatus)
	})

	t.Run("marks the job failed after the final gateway failure", func(t *testing.T) {
		ctx := context.Background()
		userID := seedUser(t)
		orderID := seedOrder(t, userID)
		p := seedPayment(t, orderID)

		repo := payment.NewPostgresRepository(testPool)
		job := &payment.Job{
			PaymentID:   p.ID,
			OrderID:     orderID,
			Action:      payment.ActionCharge,
			Status:      payment.JobStatusPending,
			MaxAttempts: 1,
			NextRetryAt: time.Now(),
		}
		require.NoError(t, repo.CreateJob(ctx, job))
		t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM payment_jobs WHERE id = $1`, job.ID) })

		svc, m := newIntegrationService(t)
		m.orderUpdater.EXPECT().MarkPaymentProcessing(mock.Anything, orderID).Return(nil)
		m.orderUpdater.EXPECT().MarkAwaitingPayment(mock.Anything, orderID).Return(nil)
		m.gw.EXPECT().Charge(mock.Anything, mock.Anything).
			Return(gateway.ChargeResponse{}, assert.AnError)

		err := svc.Process(ctx, *job)
		require.Error(t, err)

		var jobStatus string
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT status FROM payment_jobs WHERE id = $1`, job.ID).Scan(&jobStatus))
		assert.Equal(t, string(payment.JobStatusFailed), jobStatus)
	})
}
