package dashboard

import (
	"time"

	"github.com/google/uuid"
)

type SalesSummary struct {
	TotalOrders       int     `json:"total_orders"`
	TotalRevenue      int64   `json:"total_revenue"`
	AverageOrderValue float64 `json:"average_order_value"`
}

type TopProduct struct {
	ProductID uuid.UUID `json:"product_id"`
	Name      string    `json:"name"`
	TotalSold int       `json:"total_sold"`
	Revenue   int64     `json:"revenue"`
}

type RevenueData struct {
	Date       time.Time `json:"date"`
	Revenue    int64     `json:"revenue"`
	OrderCount int       `json:"order_count"`
}

type StatusBreakdown struct {
	Status string `json:"status"`
	Count  int    `json:"count"`
}
