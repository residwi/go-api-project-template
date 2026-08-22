package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type WishlistManager interface {
	Add(ctx context.Context, userID, productID uuid.UUID) error
	Remove(ctx context.Context, userID, productID uuid.UUID) error
	List(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Item, error)
}

type Handler struct {
	service   WishlistManager
	validator *validator.Validator
}

func NewHandler(service WishlistManager, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
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

func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[addItemRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.Add(r.Context(), uc.UserID, req.ProductID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, nil)
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	items, err := h.service.List(r.Context(), uc.UserID, cursor)
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

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	if err := h.service.Remove(r.Context(), uc.UserID, productID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
