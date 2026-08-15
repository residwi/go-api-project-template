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

type TopProductsReader interface {
	ListTopProducts(ctx context.Context, limit int, from, to time.Time) ([]domain.TopProduct, error)
}

type Handler struct {
	usecase TopProductsReader
}

func New(usecase TopProductsReader) *Handler {
	return &Handler{usecase: usecase}
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

	products, err := h.usecase.ListTopProducts(r.Context(), limit, from, to)
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
