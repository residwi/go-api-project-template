package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/wishlist/domain"
	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type WishlistManager interface {
	Add(ctx context.Context, userID, productID uuid.UUID) error
	Remove(ctx context.Context, userID, productID uuid.UUID) error
	List(ctx context.Context, userID uuid.UUID, cursor paging.CursorPage) ([]domain.Item, error)
}

type Handler struct {
	service WishlistManager
}

func NewHandler(service WishlistManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Add(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	req, err := request.Bind[addItemRequest](w, r)
	if err != nil {
		return err
	}

	if err := h.service.Add(r.Context(), uc.UserID, req.ProductID); err != nil {
		return err
	}

	response.Created(w, nil)
	return nil
}

func (h *Handler) List(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	cursor := paging.ParseCursorPage(r)

	items, err := h.service.List(r.Context(), uc.UserID, cursor)
	if err != nil {
		return err
	}

	out := make([]itemResponse, len(items))
	for i, it := range items {
		out[i] = toItemResponse(it)
	}

	response.CursorPage(w, out, cursor.Limit, func(it itemResponse) (time.Time, uuid.UUID) {
		return it.CreatedAt, it.ID
	})
	return nil
}

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	productID, err := request.ParseUUIDParam(r, "product_id")
	if err != nil {
		return err
	}

	if err := h.service.Remove(r.Context(), uc.UserID, productID); err != nil {
		return err
	}

	response.NoContent(w)
	return nil
}
