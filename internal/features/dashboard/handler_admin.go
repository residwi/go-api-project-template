package dashboard

import (
	"net/http"
	"strconv"
	"time"

	"github.com/residwi/go-api-project-template/internal/core/response"
)

type adminHandler struct {
	service *Service
}

func (h *adminHandler) Summary(w http.ResponseWriter, r *http.Request) {
	from, to, ok := parseDateRange(w, r)
	if !ok {
		return
	}

	sales, err := h.service.GetSalesSummary(r.Context(), from, to)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	breakdown, err := h.service.GetOrderStatusBreakdown(r.Context())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, SummaryResponse{Sales: sales, StatusBreakdown: breakdown})
}

func (h *adminHandler) TopProducts(w http.ResponseWriter, r *http.Request) {
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

	response.OK(w, products)
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

	response.OK(w, data)
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

	// Set "to" to end of day
	to = to.Add(24*time.Hour - time.Nanosecond)

	return from, to, true
}
