package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type adminHandler struct {
	service   *inventory.Service
	validator *validator.Validator
}

// Mirrors inventory.Stock 1:1. Every inventory route is admin-only, so Reserved
// is safe here -- the leak that matters is on product's public response.
type stockResponse struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int       `json:"quantity"`
	Reserved  int       `json:"reserved"`
	Available int       `json:"available"`
}

func toStockResponse(s *inventory.Stock) stockResponse {
	return stockResponse{
		ProductID: s.ProductID,
		Quantity:  s.Quantity,
		Reserved:  s.Reserved,
		Available: s.Available,
	}
}

func (h *adminHandler) GetStock(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	stock, err := h.service.GetStock(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toStockResponse(stock))
}

// Exists only to carry the validate tag: Service.Restock takes a plain int.
type restockRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

func (h *adminHandler) Restock(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[restockRequest](w, r, h.validator)
	if !ok {
		return
	}

	stock, err := h.service.Restock(r.Context(), id, req.Quantity)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toStockResponse(stock))
}

type adjustRequest struct {
	Quantity int `json:"quantity" validate:"required,min=0"`
}

func (h *adminHandler) Adjust(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[adjustRequest](w, r, h.validator)
	if !ok {
		return
	}

	stock, err := h.service.AdjustStock(r.Context(), id, req.Quantity)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toStockResponse(stock))
}
