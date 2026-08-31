package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type InventoryManager interface {
	GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error)
	Restock(ctx context.Context, productID uuid.UUID, qty int) (*domain.Stock, error)
	Adjust(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error)
}

type Handler struct {
	service InventoryManager
}

func NewHandler(service InventoryManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) GetStock(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "product_id")
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

func (h *Handler) Restock(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := request.Bind[restockRequest](w, r)
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

func (h *Handler) Adjust(w http.ResponseWriter, r *http.Request) {
	id, ok := request.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := request.Bind[adjustRequest](w, r)
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
