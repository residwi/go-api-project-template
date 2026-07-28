package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/product"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type updateProductRequest struct {
	CategoryID     *uuid.UUID `json:"category_id" validate:"omitempty"`
	Name           *string    `json:"name" validate:"omitempty,min=1,max=255"`
	Description    *string    `json:"description" validate:"omitempty"`
	Price          *int64     `json:"price" validate:"omitempty,min=0"`
	CompareAtPrice *int64     `json:"compare_at_price" validate:"omitempty,min=0"`
	Currency       *string    `json:"currency" validate:"omitempty,len=3"`
	SKU            *string    `json:"sku" validate:"omitempty,max=100"`
	Status         *string    `json:"status" validate:"omitempty,oneof=draft published archived"`
}

func (r updateProductRequest) toUpdateParams() product.UpdateParams {
	return product.UpdateParams{
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

func (h *adminHandler) Update(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[updateProductRequest](w, r, h.validator)
	if !ok {
		return
	}

	p, err := h.service.Update(r.Context(), id, req.toUpdateParams())
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toAdminProductResponse(p))
}
