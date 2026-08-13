package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/retrypayment"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type Command interface {
	Execute(ctx context.Context, userID, orderID uuid.UUID, p retrypayment.Params) (*retrypayment.Result, error)
}

type Handler struct {
	cmd       Command
	validator *validator.Validator
}

func New(cmd Command, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
}

func (h *Handler) RegisterHTTP(authed *middleware.RouteGroup, limiter middleware.Middleware) {
	handler := http.HandlerFunc(h.retry)
	if limiter != nil {
		authed.Handle("POST /orders/{id}/pay", limiter(handler))
		return
	}
	authed.Handle("POST /orders/{id}/pay", handler)
}

type payRequest struct {
	PaymentMethodID string `json:"payment_method_id" validate:"required"`
}

type payResultResponse struct {
	PaymentID  uuid.UUID `json:"payment_id"`
	PaymentURL string    `json:"payment_url,omitempty"`
	Charged    bool      `json:"charged"`
}

func toPayResultResponse(r *retrypayment.Result) payResultResponse {
	return payResultResponse{
		PaymentID:  r.PaymentID,
		PaymentURL: r.PaymentURL,
		Charged:    r.Charged,
	}
}

func (h *Handler) retry(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := response.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := response.Bind[payRequest](w, r, h.validator)
	if !ok {
		return
	}

	result, err := h.cmd.Execute(r.Context(), uc.UserID, id, retrypayment.Params{PaymentMethodID: req.PaymentMethodID})
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toPayResultResponse(result))
}
