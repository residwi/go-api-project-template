package http

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/checkout"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/platform/web/request"
	"github.com/residwi/go-api-project-template/internal/platform/web/response"
)

type CheckoutManager interface {
	PlaceOrder(
		ctx context.Context,
		userID uuid.UUID,
		in checkout.PlaceOrderInput,
	) (*order.Snapshot, error)
	RetryPayment(
		ctx context.Context, userID, orderID uuid.UUID, paymentMethodID string,
	) (payment.ChargeResult, error)
	CancelOrder(ctx context.Context, userID, orderID uuid.UUID) error
}

type Handler struct {
	service CheckoutManager
}

func NewHandler(service CheckoutManager) *Handler {
	return &Handler{service: service}
}

func (h *Handler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	uc, ok := request.RequireUser(w, r)
	if !ok {
		return
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		response.BadRequest(w, "Idempotency-Key header is required")
		return
	}

	req, ok := request.Bind[placeOrderRequest](w, r)
	if !ok {
		return
	}

	in := checkout.PlaceOrderInput{
		Order:           req.toOrder(),
		PaymentMethodID: req.PaymentMethodID,
		IdempotencyKey:  idempotencyKey,
	}

	placed, err := h.service.PlaceOrder(r.Context(), uc.UserID, in)
	if err != nil {
		response.HandleErr(w, err)
		return
	}

	response.Created(w, toPlaceOrderResponse(placed))
}

func (h *Handler) RetryPayment(w http.ResponseWriter, r *http.Request) {
	uc, ok := request.RequireUser(w, r)
	if !ok {
		return
	}

	id, ok := request.ParseUUIDParam(w, r, "id")
	if !ok {
		return
	}

	req, ok := request.Bind[payRequest](w, r)
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

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	uc, ok := request.RequireUser(w, r)
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
