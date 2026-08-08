package dashboard

import "github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"

// Aliases keep service.go, postgres/ and http/ compiling while the slices are
// extracted one at a time. Deleted along with the husk.
type (
	SalesSummary    = domain.SalesSummary
	TopProduct      = domain.TopProduct
	RevenueData     = domain.RevenueData
	StatusBreakdown = domain.StatusBreakdown
)
