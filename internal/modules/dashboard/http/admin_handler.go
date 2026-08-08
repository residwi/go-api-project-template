package http

import (
	"net/http"
	"time"

	"github.com/residwi/go-api-project-template/internal/modules/dashboard"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service *dashboard.Service
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

func (h *adminHandler) Revenue(w http.ResponseWriter, r *http.Request) {
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
