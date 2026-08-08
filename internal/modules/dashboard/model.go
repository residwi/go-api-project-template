package dashboard

import "github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"

// Aliases keep service.go, postgres/ and http/ compiling while the slices are
// extracted one at a time. Deleted along with the husk.
type (
	// SalesSummary aliases domain.SalesSummary.
	SalesSummary = domain.SalesSummary
	// TopProduct aliases domain.TopProduct.
	TopProduct = domain.TopProduct
	// RevenueData aliases domain.RevenueData.
	RevenueData = domain.RevenueData
	// StatusBreakdown aliases domain.StatusBreakdown.
	StatusBreakdown = domain.StatusBreakdown
)
