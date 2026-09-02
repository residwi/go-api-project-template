package http

import (
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/dashboard/domain"
)

type salesSummaryResponse struct {
	TotalOrders       int     `json:"total_orders"`
	TotalRevenue      int64   `json:"total_revenue"`
	AverageOrderValue float64 `json:"average_order_value"`
}

type statusBreakdownResponse struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type summaryResponse struct {
	Sales           salesSummaryResponse      `json:"sales"`
	StatusBreakdown []statusBreakdownResponse `json:"status_breakdown"`
}

func toSummaryResponse(sales domain.SalesSummary, breakdown []domain.StatusBreakdown) summaryResponse {
	sb := make([]statusBreakdownResponse, len(breakdown))
	for i, b := range breakdown {
		sb[i] = statusBreakdownResponse{Status: b.Status, Count: b.Count}
	}

	return summaryResponse{
		Sales: salesSummaryResponse{
			TotalOrders:       sales.TotalOrders,
			TotalRevenue:      sales.TotalRevenue,
			AverageOrderValue: sales.AverageOrderValue,
		},
		StatusBreakdown: sb,
	}
}

type revenueDataResponse struct {
	Date       time.Time `json:"date"`
	Revenue    int64     `json:"revenue"`
	OrderCount int       `json:"order_count"`
}

func toRevenueDataResponse(d domain.RevenueData) revenueDataResponse {
	return revenueDataResponse{
		Date:       d.Date,
		Revenue:    d.Revenue,
		OrderCount: d.OrderCount,
	}
}

type topProductResponse struct {
	ProductID uuid.UUID `json:"product_id"`
	Name      string    `json:"name"`
	TotalSold int       `json:"total_sold"`
	Revenue   int64     `json:"revenue"`
}

func toTopProductResponse(p domain.TopProduct) topProductResponse {
	return topProductResponse{
		ProductID: p.ProductID,
		Name:      p.Name,
		TotalSold: p.TotalSold,
		Revenue:   p.Revenue,
	}
}
