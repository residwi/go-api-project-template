package http

import (
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/order"
	"github.com/residwi/go-api-project-template/internal/transport/http/middleware"
	"github.com/residwi/go-api-project-template/internal/transport/http/response"
)

// payRequest has no params.go counterpart: order.Service.RetryPayment
// already takes a plain string, not a request struct, so there is no
// dto-in-the-core cycle to break here.
type payRequest struct {
	PaymentMethodID string `json:"payment_method_id" validate:"required"`
}

// payResultResponse gives order.PaymentResult proper snake_case wire tags --
// the pre-refactor handler serialized it untagged (ports.go carries no json
// tags at all), so this also fixes a latent inconsistency with every other
// response in the app, not just a rename.
type payResultResponse struct {
	PaymentID  uuid.UUID `json:"payment_id"`
	PaymentURL string    `json:"payment_url,omitempty"`
	Charged    bool      `json:"charged"`
}

func toPayResultResponse(r *order.PaymentResult) payResultResponse {
	return payResultResponse{
		PaymentID:  r.PaymentID,
		PaymentURL: r.PaymentURL,
		Charged:    r.Charged,
	}
}

func (h *publicHandler) RetryPayment(w http.ResponseWriter, r *http.Request) {
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
