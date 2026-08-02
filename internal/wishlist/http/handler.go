package http

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/platform/paging"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
	"github.com/residwi/go-api-project-template/internal/wishlist"
)

type handler struct {
	service   *wishlist.Service
	validator *validator.Validator
}

// itemResponse is this endpoint's wire contract. It is unexported and lives here
// rather than on wishlist.Item so that adding a field to the domain model does
// not publish it.
type itemResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	CreatedAt time.Time `json:"created_at"`
}

// toItemResponse is the explicit mapping. WishlistID is deliberately dropped: it
// is an internal join key and a client has no use for it.
func toItemResponse(it wishlist.Item) itemResponse {
	return itemResponse{
		ID:        it.ID,
		ProductID: it.ProductID,
		CreatedAt: it.CreatedAt,
	}
}

func (h *handler) GetWishlist(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	cursor := paging.ParseCursorPage(r)

	items, err := h.service.GetWishlist(r.Context(), uc.UserID, cursor)
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

// addItemRequest carries the validation rules. They live here, not in the core:
// a service called from a worker should not inherit HTTP's validation vocabulary.
type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
}

func (h *handler) AddItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := response.Bind[addItemRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.AddItem(r.Context(), uc.UserID, req.ProductID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, nil)
}

func (h *handler) RemoveItem(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	if err := h.service.RemoveItem(r.Context(), uc.UserID, productID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
