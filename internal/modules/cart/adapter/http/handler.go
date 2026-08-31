package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
)

type CartManager interface {
	Get(ctx context.Context, userID uuid.UUID) (*domain.Cart, error)
	Add(ctx context.Context, userID, productID uuid.UUID, quantity int) error
	UpdateQuantity(ctx context.Context, userID, productID uuid.UUID, quantity int) error
	Remove(ctx context.Context, userID, productID uuid.UUID) error
	Clear(ctx context.Context, userID uuid.UUID) error
}

type Handler struct {
	service   CartManager
	validator *validator.Validator
}

func NewHandler(service CartManager, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	c, err := h.service.Get(r.Context(), uc.UserID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	out, err := toCartResponse(c)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, out)
}

func (h *Handler) Add(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	req, ok := request.Bind[addItemRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.Add(r.Context(), uc.UserID, req.ProductID, req.Quantity); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, nil)
}

func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := request.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := request.Bind[updateQuantityRequest](w, r, h.validator)
	if !ok {
		return
	}

	if err := h.service.UpdateQuantity(r.Context(), uc.UserID, productID, req.Quantity); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *Handler) Remove(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	productID, ok := request.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	if err := h.service.Remove(r.Context(), uc.UserID, productID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}

func (h *Handler) Clear(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	if err := h.service.Clear(r.Context(), uc.UserID); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
