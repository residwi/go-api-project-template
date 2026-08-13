package http

import (
	"context"
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type SummaryReader interface {
	GetSummary(ctx context.Context, from, to time.Time) (domain.SalesSummary, []domain.StatusBreakdown, error)
}

type Handler struct {
	reader SummaryReader
}

func New(reader SummaryReader) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("GET /dashboard/summary", h.summary)
}

func parseDateRange(w http.ResponseWriter, r *http.Request) (from, to time.Time, ok bool) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		response.BadRequest(w, "from and to query parameters are required")
		return time.Time{}, time.Time{}, false
	}

	from, err := time.Parse("2006-01-02", fromStr)
	if err != nil {
		response.BadRequest(w, "invalid from date format, expected YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}

	to, err = time.Parse("2006-01-02", toStr)
	if err != nil {
		response.BadRequest(w, "invalid to date format, expected YYYY-MM-DD")
		return time.Time{}, time.Time{}, false
	}

	to = to.Add(24*time.Hour - time.Nanosecond)

	return from, to, true
}

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

func (h *Handler) summary(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	sales, breakdown, err := h.reader.GetSummary(r.Context(), from, to)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toSummaryResponse(sales, breakdown))
}
