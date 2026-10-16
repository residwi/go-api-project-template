package http

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/residwi/go-api-project-template/internal/features/dashboard/domain"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
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

func parseDateRange(r *http.Request) (from, to time.Time, err error) {
	fromStr := r.URL.Query().Get("from")
	toStr := r.URL.Query().Get("to")

	if fromStr == "" || toStr == "" {
		return time.Time{}, time.Time{}, fmt.Errorf("%w: from and to query parameters are required", errs.ErrBadRequest)
	}

	from, err = time.Parse("2006-01-02", fromStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf(
			"%w: invalid from date format, expected YYYY-MM-DD",
			errs.ErrBadRequest,
		)
	}

	to, err = time.Parse("2006-01-02", toStr)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf(
			"%w: invalid to date format, expected YYYY-MM-DD",
			errs.ErrBadRequest,
		)
	}

	to = to.Add(24*time.Hour - time.Nanosecond)

	return from, to, nil
}

func (h *Handler) GetSummary(w http.ResponseWriter, r *http.Request) error {
	from, to, err := parseDateRange(r)
	if err != nil {
		return err
	}

	sales, breakdown, err := h.service.GetSummary(r.Context(), from, to)
	if err != nil {
		return err
	}

	response.OK(w, toSummaryResponse(sales, breakdown))
	return nil
}

func (h *Handler) ListRevenueByDay(w http.ResponseWriter, r *http.Request) error {
	from, to, err := parseDateRange(r)
	if err != nil {
		return err
	}

	data, err := h.service.ListRevenueByDay(r.Context(), from, to)
	if err != nil {
		return err
	}

	out := make([]revenueDataResponse, len(data))
	for i, d := range data {
		out[i] = toRevenueDataResponse(d)
	}

	response.OK(w, out)
	return nil
}

func (h *Handler) ListTopProducts(w http.ResponseWriter, r *http.Request) error {
	from, to, err := parseDateRange(r)
	if err != nil {
		return err
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit < 1 || limit > 100 {
		limit = 10
	}

	products, err := h.service.ListTopProducts(r.Context(), limit, from, to)
	if err != nil {
		return err
	}

	out := make([]topProductResponse, len(products))
	for i, p := range products {
		out[i] = toTopProductResponse(p)
	}

	response.OK(w, out)
	return nil
}
