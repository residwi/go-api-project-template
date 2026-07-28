package http

import (
	"net/http"

	"github.com/google/uuid"

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

func (r createProductRequest) toCreateParams() product.CreateParams {
	return product.CreateParams{
		CategoryID:     r.CategoryID,
		Name:           r.Name,
		Description:    r.Description,
		Price:          r.Price,
		CompareAtPrice: r.CompareAtPrice,
		Currency:       r.Currency,
		SKU:            r.SKU,
		Status:         r.Status,
	}
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
