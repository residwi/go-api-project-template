package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/modules/checkout"
	orderdomain "github.com/residwi/go-api-project-template/internal/modules/order/domain"
	"github.com/residwi/go-api-project-template/internal/modules/payment"
	"github.com/residwi/go-api-project-template/internal/platform/validator"
	"github.com/residwi/go-api-project-template/internal/platform/web/middleware"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type Checkout interface {
	PlaceOrder(
		ctx context.Context,
		userID uuid.UUID,
		in checkout.PlaceOrderInput,
	) (*orderdomain.Order, error)
	RetryPayment(
		ctx context.Context, userID, orderID uuid.UUID, paymentMethodID string,
	) (payment.ChargeResult, error)
	CancelOrder(ctx context.Context, userID, orderID uuid.UUID) error
}

type Handler struct {
	service   Checkout
	validator *validator.Validator
}

func NewHandler(service Checkout, v *validator.Validator) *Handler {
	return &Handler{service: service, validator: v}
}

func (h *Handler) Place(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		response.BadRequest(w, "Idempotency-Key header is required")
		return
	}

	req, ok := request.Bind[placeOrderRequest](w, r, h.validator)
	if !ok {
		return
	}

	in := checkout.PlaceOrderInput{
		Order:           req.toOrder(),
		PaymentMethodID: req.PaymentMethodID,
		IdempotencyKey:  idempotencyKey,
	}

	order, err := h.service.PlaceOrder(r.Context(), uc.UserID, in)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toPlaceOrderResponse(order))
}

func (h *Handler) Retry(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := request.Bind[payRequest](w, r, h.validator)
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

func (h *Handler) Cancel(w http.ResponseWriter, r *http.Request) {
	uc, ok := middleware.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	if err := h.service.CancelOrder(r.Context(), uc.UserID, id); err != nil {
		response.HandleErr(w, err)
		return
	}

	response.NoContent(w)
}
