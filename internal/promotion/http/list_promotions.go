package http

import (
	"net/http"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/promotion"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

func (h *adminHandler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := promotion.ListParams{
		Page:     page.Page,
		PageSize: page.PageSize,
	}

	promotions, total, err := h.service.List(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]adminPromotionResponse, len(promotions))
	for i, p := range promotions {
		out[i] = toAdminPromotionResponse(&p)
	}

	response.Paginated(w, paging.NewOffsetPageResult(out, page, total))
}
