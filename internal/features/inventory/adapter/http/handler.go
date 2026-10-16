package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/inventory/domain"
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

func (h *Handler) GetStock(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "product_id")
	if err != nil {
		return err
	}

	stock, err := h.service.GetStock(r.Context(), id)
	if err != nil {
		return err
	}

	response.OK(w, toStockResponse(stock))
	return nil
}

func (h *Handler) Restock(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "product_id")
	if err != nil {
		return err
	}

	req, err := request.Bind[restockRequest](w, r)
	if err != nil {
		return err
	}

	stock, err := h.service.Restock(r.Context(), id, req.Quantity)
	if err != nil {
		return err
	}

	response.OK(w, toStockResponse(stock))
	return nil
}

func (h *Handler) Adjust(w http.ResponseWriter, r *http.Request) error {
	id, err := request.ParseUUIDParam(r, "product_id")
	if err != nil {
		return err
	}

	req, err := request.Bind[adjustRequest](w, r)
	if err != nil {
		return err
	}

	stock, err := h.service.Adjust(r.Context(), id, req.Quantity)
	if err != nil {
		return err
	}

	response.OK(w, toStockResponse(stock))
	return nil
}
