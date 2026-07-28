package http

import (
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/dashboard"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

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
