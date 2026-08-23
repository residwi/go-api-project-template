package http

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/cart/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/server/middleware"
	"github.com/residwi/go-api-project-template/internal/server/response"
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

type addItemRequest struct {
	ProductID uuid.UUID `json:"product_id" validate:"required"`
	Quantity  int       `json:"quantity"   validate:"required,min=1"`
}

type updateQuantityRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

type cartResponse struct {
	ID    uuid.UUID          `json:"id"`
	Items []cartItemResponse `json:"items"`
	Total int64              `json:"total"`
}

type cartItemResponse struct {
	ID        uuid.UUID `json:"id"`
	ProductID uuid.UUID `json:"product_id"`
	Name      string    `json:"name"`
	Price     int64     `json:"price"`
	Currency  string    `json:"currency"`
	Quantity  int       `json:"quantity"`
	Available int       `json:"available_stock"`
	Sellable  bool      `json:"sellable"`
	CreatedAt time.Time `json:"created_at"`
}

func toCartResponse(c *domain.Cart) (cartResponse, error) {
	out := cartResponse{ID: c.ID, Items: make([]cartItemResponse, len(c.Items))}
	for i, it := range c.Items {
		out.Items[i] = cartItemResponse{
			ID:        it.ID,
			ProductID: it.ProductID,
			Name:      it.Product.Name,
			Price:     it.Product.Price.Amount,
			Currency:  it.Product.Price.Currency,
			Quantity:  it.Quantity,
			Available: it.Product.Stock,
			Sellable:  it.Product.Sellable(),
			CreatedAt: it.CreatedAt,
		}
	}

	total, err := c.Total()
	if err != nil {
		return cartResponse{}, err
	}
	out.Total = total.Amount
	return out, nil
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

	req, ok := response.Bind[addItemRequest](w, r, h.validator)
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

	productID, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateQuantityRequest](w, r, h.validator)
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
