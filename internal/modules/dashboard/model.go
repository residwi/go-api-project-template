package dashboard

import (
	"time"

	"github.com/google/uuid"
)

type SalesSummary struct {
	TotalOrders       int
	TotalRevenue      int64
	AverageOrderValue float64
}

type TopProduct struct {
	ProductID uuid.UUID
	Name      string
	TotalSold int
	Revenue   int64
}

type RevenueData struct {
	Date       time.Time
	Revenue    int64
	OrderCount int
}

type StatusBreakdown struct {
	Status string
	Count  int
}
