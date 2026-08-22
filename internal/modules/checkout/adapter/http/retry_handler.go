package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	paymentcontract "github.com/residwi/go-api-project-template/internal/modules/payment/contract"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

type PaymentRetrier interface {
	RetryPayment(
		ctx context.Context, userID, orderID uuid.UUID, paymentMethodID string,
	) (paymentcontract.ChargeResult, error)
}

type RetryHandler struct {
	service   PaymentRetrier
	validator *validator.Validator
}

func NewRetryHandler(service PaymentRetrier, v *validator.Validator) *RetryHandler {
	return &RetryHandler{service: service, validator: v}
}

type payRequest struct {
	PaymentMethodID string `json:"payment_method_id" validate:"required"`
}

type payResultResponse struct {
	PaymentID  uuid.UUID `json:"payment_id"`
	PaymentURL string    `json:"payment_url,omitempty"`
	Charged    bool      `json:"charged"`
}

func toPayResultResponse(r paymentcontract.ChargeResult) payResultResponse {
	return payResultResponse{
		PaymentID:  r.PaymentID,
		PaymentURL: r.PaymentURL,
		Charged:    r.Charged,
	}
}

func (h *RetryHandler) Retry(w http.ResponseWriter, r *http.Request) {
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

	result, err := h.service.RetryPayment(r.Context(), uc.UserID, id, req.PaymentMethodID)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.OK(w, toPayResultResponse(result))
}
