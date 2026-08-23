package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/server/response"
)

// InventoryManager is everything this admin-only handler needs from
// inventory.Service -- read, restock and adjust together, since one role
// fields all three routes.
type InventoryManager interface {
	GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error)
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
	Adjust(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error)
}

type Handler struct {
	service   InventoryManager
	validator *validator.Validator
}

func NewHandler(service InventoryManager, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

type stockResponse struct {
	ProductID uuid.UUID `json:"product_id"`
	Quantity  int       `json:"quantity"`
	Reserved  int       `json:"reserved"`
	Available int       `json:"available"`
}

func toStockResponse(s *domain.Stock) stockResponse {
	return stockResponse{
		ProductID: s.ProductID,
		Quantity:  s.Quantity,
		Reserved:  s.Reserved,
		Available: s.Available,
	}
}

func (h *Handler) GetStock(w http.ResponseWriter, r *http.Request) {
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

type restockRequest struct {
	Quantity int `json:"quantity" validate:"required,min=1"`
}

func (h *Handler) Restock(w http.ResponseWriter, r *http.Request) {
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

func (h *Handler) Adjust(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[adjustRequest](w, r, h.validator)
	if !ok {
		return
	}

	stock, err := h.service.Adjust(r.Context(), id, req.Quantity)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toStockResponse(stock))
}
