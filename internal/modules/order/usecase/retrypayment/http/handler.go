package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/order/usecase/retrypayment"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type UseCase interface {
	Execute(ctx context.Context, userID, orderID uuid.UUID, p retrypayment.Params) (*retrypayment.Result, error)
}

type Handler struct {
	cmd       UseCase
	validator *validator.Validator
}

func New(cmd UseCase, v *validator.Validator) *Handler {
	return &Handler{cmd: cmd, validator: v}
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

func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
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
