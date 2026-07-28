package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/dashboard"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// salesSummaryResponse and statusBreakdownResponse mirror their domain
// counterparts 1:1 -- dashboard is the reporting read-model, so its own
// types are already shaped for the admin UI rather than for some other
// consumer, and there is nothing on either to omit.
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

func toSummaryResponse(sales dashboard.SalesSummary, breakdown []dashboard.StatusBreakdown) summaryResponse {
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

func (h *handler) Summary(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	sales, breakdown, err := h.service.GetSummary(r.Context(), from, to)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toSummaryResponse(sales, breakdown))
}
