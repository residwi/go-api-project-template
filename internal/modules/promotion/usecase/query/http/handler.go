package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/promotion/domain"
	"github.com/residwi/go-api-project-template/internal/modules/promotion/usecase/query"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type PromotionLister interface {
	ListAdmin(ctx context.Context, params query.Params) ([]domain.Promotion, int, error)
}

type Handler struct {
	reader PromotionLister
}

func New(reader PromotionLister) *Handler {
	return &Handler{reader: reader}
}

type adminPromotionResponse struct {
	ID             uuid.UUID   `json:"id"`
	Code           string      `json:"code"`
	Type           domain.Type `json:"type"`
	Value          int64       `json:"value"`
	MinOrderAmount int64       `json:"min_order_amount"`
	MaxDiscount    *int64      `json:"max_discount,omitempty"`
	MaxUses        *int        `json:"max_uses,omitempty"`
	UsedCount      int         `json:"used_count"`
	StartsAt       time.Time   `json:"starts_at"`
	ExpiresAt      time.Time   `json:"expires_at"`
	Active         bool        `json:"active"`
	CreatedAt      time.Time   `json:"created_at"`
	UpdatedAt      time.Time   `json:"updated_at"`
}

func toAdminPromotionResponse(p *domain.Promotion) adminPromotionResponse {
	return adminPromotionResponse{
		ID:             p.ID,
		Code:           p.Code,
		Type:           p.Type,
		Value:          p.Value,
		MinOrderAmount: p.MinOrderAmount,
		MaxDiscount:    p.MaxDiscount,
		MaxUses:        p.MaxUses,
		UsedCount:      p.UsedCount,
		StartsAt:       p.StartsAt,
		ExpiresAt:      p.ExpiresAt,
		Active:         p.Active,
		CreatedAt:      p.CreatedAt,
		UpdatedAt:      p.UpdatedAt,
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	page := paging.ParseOffsetPage(r)
	params := query.Params{OffsetPage: page}

	promotions, total, err := h.reader.ListAdmin(r.Context(), params)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]adminPromotionResponse, len(promotions))
	for i, p := range promotions {
		out[i] = toAdminPromotionResponse(&p)
	}

	response.OK(w, paging.NewOffsetPageResult(out, page, total))
}
