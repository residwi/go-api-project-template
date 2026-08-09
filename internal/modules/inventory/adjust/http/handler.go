package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/inventory/domain"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// Adjuster is what Handler needs from adjust.Command: adjust.Command
// satisfies it directly, so nothing sits between them, and the
// mockery-generated mock is the other implementation, used in handler_test.go.
type Adjuster interface {
	Execute(ctx context.Context, productID uuid.UUID, newQuantity int) (*domain.Stock, error)
}

type Handler struct {
	cmd       Adjuster
	validator *validator.Validator
}

func New(cmd Adjuster, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(admin *middleware.RouteGroup) {
	admin.HandleFunc("PUT /inventory/{product_id}/adjust", h.adjust)
}

// Declared here, not shared with inventory's other slices, so one endpoint's
// new field cannot appear in another's response.
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

type adjustRequest struct {
	Quantity int `json:"quantity" validate:"required,min=0"`
}

func (h *Handler) adjust(w http.ResponseWriter, r *http.Request) {
	id, ok := response.ParseUUIDParam(w, r, "product_id")
	if !ok {
		return
	}

	req, ok := response.Bind[adjustRequest](w, r, h.validator)
	if !ok {
		return
	}

	stock, err := h.cmd.Execute(r.Context(), id, req.Quantity)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toStockResponse(stock))
}
