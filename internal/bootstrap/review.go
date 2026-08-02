package bootstrap

import (
	"context"

	"github.com/residwi/go-api-project-template/internal/modules/order"
	"github.com/residwi/go-api-project-template/internal/modules/review"
)

// purchaseVerifierAdapter maps review's locally-declared DeliveredPurchase onto
// order's DeliveredPurchaseParams. The types are identical by design: each
// module names the fields it needs rather than importing the other's package.
type purchaseVerifierAdapter struct{ svc *order.Service }

func (a *purchaseVerifierAdapter) HasDeliveredOrder(ctx context.Context, p review.DeliveredPurchase) (bool, error) {
	return a.svc.HasDeliveredOrder(ctx, order.DeliveredPurchaseParams{
		UserID:    p.UserID,
		OrderID:   p.OrderID,
		ProductID: p.ProductID,
	})
}

func NewReviewService(repo review.Repository, orderSvc *order.Service) *review.Service {
	return review.NewService(repo, &purchaseVerifierAdapter{svc: orderSvc})
}
