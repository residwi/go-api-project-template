package http

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/platform/response"
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
