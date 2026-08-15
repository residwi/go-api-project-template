package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type ItemReader interface {
	ListItemsForUser(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Item, error)
}

type Handler struct {
	usecase ItemReader
}

func New(usecase ItemReader) *Handler {
	return &Handler{usecase: usecase}
}

type itemResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
}

func toItemResponse(it domain.Item) itemResponse {
	return itemResponse{
		ID:        it.ID,
		ProductID: it.ProductID,
		CreatedAt: it.CreatedAt,
	}
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	items, err := h.usecase.ListItemsForUser(r.Context(), uc.UserID, cursor)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out := make([]itemResponse, len(items))
	for i, it := range items {
		out[i] = toItemResponse(it)
	}

	response.CursorPage(w, out, cursor.Limit, func(it itemResponse) (time.Time, uuid.UUID) {
		return it.CreatedAt, it.ID
	})
}
