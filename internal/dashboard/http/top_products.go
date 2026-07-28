package http

import (
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/dashboard"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

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
