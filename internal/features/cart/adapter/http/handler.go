package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/cart/domain"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type CartManager interface {
	Get(ctx context.Context, userID uuid.UUID) (*domain.Cart, error)
	Add(ctx context.Context, userID, productID uuid.UUID, quantity int) error
	UpdateQuantity(ctx context.Context, userID, productID uuid.UUID, quantity int) error
	Remove(ctx context.Context, userID, productID uuid.UUID) error
	Clear(ctx context.Context, userID uuid.UUID) error
}

type Handler struct {
	service CartManager
}

func NewHandler(service CartManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Get(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	c, err := h.service.Get(r.Context(), uc.UserID)
	if err != nil {
		return err
	}

	out, err := toCartResponse(c)
	if err != nil {
		return err
	}

	response.OK(w, out)
	return nil
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

	if err := h.service.Add(r.Context(), uc.UserID, req.ProductID, req.Quantity); err != nil {
		return err
	}

	response.Created(w, nil)
	return nil
}

func (h *Handler) UpdateQuantity(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	productID, err := request.ParseUUIDParam(r, "product_id")
	if err != nil {
		return err
	}

	req, err := request.Bind[updateQuantityRequest](w, r)
	if err != nil {
		return err
	}

	if err := h.service.UpdateQuantity(r.Context(), uc.UserID, productID, req.Quantity); err != nil {
		return err
	}

	response.NoContent(w)
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

func (h *Handler) Clear(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	if err := h.service.Clear(r.Context(), uc.UserID); err != nil {
		return err
	}

	response.NoContent(w)
	return nil
}
