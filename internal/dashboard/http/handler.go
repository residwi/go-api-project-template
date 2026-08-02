package http

import (
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/dashboard"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type handler struct {
	service *dashboard.Service
}

// parseDateRange is shared by all three endpoints, not any one of them, so
// it lives here rather than being duplicated or arbitrarily owned by one
// endpoint's section of this file.
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

	// Set "to" to end of day
	to = to.Add(24*time.Hour - time.Nanosecond)

	return from, to, true
}

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

type topProductResponse struct {
	ProductID uuid.UUID `json:"product_id"`
	Name      string    `json:"name"`
	TotalSold int       `json:"total_sold"`
	Revenue   int64     `json:"revenue"`
}

func toTopProductResponse(p dashboard.TopProduct) topProductResponse {
	return topProductResponse{
		ProductID: p.ProductID,
		Name:      p.Name,
		TotalSold: p.TotalSold,
		Revenue:   p.Revenue,
	}
}

func (h *handler) TopProducts(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 10
	}

	products, err := h.service.GetTopProducts(r.Context(), limit, from, to)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]topProductResponse, len(products))
	for i, p := range products {
		out[i] = toTopProductResponse(p)
	}

	response.OK(w, out)
}

type revenueDataResponse struct {
	Date       time.Time `json:"date"`
	Revenue    int64     `json:"revenue"`
	OrderCount int       `json:"order_count"`
}

func toRevenueDataResponse(d dashboard.RevenueData) revenueDataResponse {
	return revenueDataResponse{
		Date:       d.Date,
		Revenue:    d.Revenue,
		OrderCount: d.OrderCount,
	}
}

func (h *handler) Revenue(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	data, err := h.service.GetRevenueByDay(r.Context(), from, to)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]revenueDataResponse, len(data))
	for i, d := range data {
		out[i] = toRevenueDataResponse(d)
	}

	response.OK(w, out)
}
