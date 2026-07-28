package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/money"
	"github.com/residwi/go-api-project-template/internal/product"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type createProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id" validate:"omitempty"`
	Name           string     `json:"name" validate:"required,min=1,max=255"`
	Description    *string    `json:"description" validate:"omitempty"`
	Price          int64      `json:"price" validate:"required,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       string     `json:"currency" validate:"omitempty,len=3"`
	SKU            *string    `json:"sku" validate:"omitempty,max=100"`
	Status         string     `json:"status" validate:"omitempty,oneof=draft published archived"`
}

// toCreateParams pairs the request's two amounts with its single `currency`
// key, so both arrive at the service denominated and agreeing with each other.
//
// `currency` is optional on the wire and stays so: an empty currency reaches
// product.Service.Create, which denominates the product in its default. The
// default is a business rule, not a transport one, so it is not applied here.
func (r createProductRequest) toCreateParams() product.CreateParams {
	p := product.CreateParams{
		CategoryID:  r.CategoryID,
		Name:        r.Name,
		Description: r.Description,
		Price:       money.New(r.Price, r.Currency),
		SKU:         r.SKU,
		Status:      r.Status,
	}
	if r.CompareAtPrice != nil {
		compareAt := money.New(*r.CompareAtPrice, r.Currency)
		p.CompareAtPrice = &compareAt
	}
	return p
}

func (h *adminHandler) Create(w http.ResponseWriter, r *http.Request) {
	req, ok := response.Bind[createProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	p, err := h.service.Create(r.Context(), req.toCreateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toAdminProductResponse(p))
}
