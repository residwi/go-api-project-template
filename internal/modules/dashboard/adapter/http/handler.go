package http

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type Reporter interface {
	ListRevenueByDay(ctx context.Context, from, to time.Time) ([]domain.RevenueData, error)
	GetSummary(ctx context.Context, from, to time.Time) (domain.SalesSummary, []domain.StatusBreakdown, error)
	ListTopProducts(ctx context.Context, limit int, from, to time.Time) ([]domain.TopProduct, error)
}

type Handler struct {
	service Reporter
}

func NewHandler(service Reporter) *Handler {
	return &Handler{service: service}
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

func (h *Handler) Summary(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) Revenue(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	data, err := h.service.ListRevenueByDay(r.Context(), from, to)
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

func (h *Handler) TopProducts(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 10
	}

	products, err := h.service.ListTopProducts(r.Context(), limit, from, to)
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
