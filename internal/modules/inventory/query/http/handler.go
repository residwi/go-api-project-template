package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// StockReader is what Handler needs from query.Reader: query.Reader satisfies
// it directly, so nothing sits between them, and the mockery-generated mock is
// the other implementation, used in handler_test.go.
type StockReader interface {
	GetStock(ctx context.Context, productID uuid.UUID) (*domain.Stock, error)
}

type Handler struct {
	reader StockReader
}

func New(reader StockReader) *Handler {
	return &Handler{reader: reader}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("GET /inventory/{product_id}", h.getStock)
}

// Mirrors domain.Stock 1:1. Every inventory route is admin-only, so Reserved
// is safe here -- the leak that matters is on product's public response.
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

func (h *Handler) getStock(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	stock, err := h.reader.GetStock(r.Context(), id)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toStockResponse(stock))
}
