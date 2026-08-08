// Package domain holds dashboard's read-model result shapes. It is
// module-private: no other module may import it. dashboard queries tables it
// does not own -- ARCHITECTURE.md's reporting-read-model carve-out -- so these
// are report shapes with no rules of their own, not an owned aggregate; each
// slice still returns its own through its Reader.
package domain

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
