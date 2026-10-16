package http

import (
	"context"
	"fmt"
	"net/http"

	"github.com/google/uuid"

	"github.com/residwi/go-api-project-template/internal/features/checkout"
	"github.com/residwi/go-api-project-template/internal/features/order"
	"github.com/residwi/go-api-project-template/internal/features/payment"
	"github.com/residwi/go-api-project-template/internal/platform/errs"
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

func (h *Handler) PlaceOrder(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	idempotencyKey := r.Header.Get("Idempotency-Key")
	if idempotencyKey == "" {
		return fmt.Errorf("%w: Idempotency-Key header is required", errs.ErrBadRequest)
	}

	req, err := request.Bind[placeOrderRequest](w, r)
	if err != nil {
		return err
	}

	in := checkout.PlaceOrderInput{
		Order:           req.toOrder(),
		PaymentMethodID: req.PaymentMethodID,
		IdempotencyKey:  idempotencyKey,
	}

	placed, err := h.service.PlaceOrder(r.Context(), uc.UserID, in)
	if err != nil {
		return err
	}

	response.Created(w, toPlaceOrderResponse(placed))
	return nil
}

func (h *Handler) RetryPayment(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	req, err := request.Bind[payRequest](w, r)
	if err != nil {
		return err
	}

	result, err := h.service.RetryPayment(r.Context(), uc.UserID, id, req.PaymentMethodID)
	if err != nil {
		return err
	}

	response.OK(w, toPayResultResponse(result))
	return nil
}

func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) error {
	uc, err := request.RequireUser(r)
	if err != nil {
		return err
	}

	id, err := request.ParseUUIDParam(r, "id")
	if err != nil {
		return err
	}

	if err := h.service.CancelOrder(r.Context(), uc.UserID, id); err != nil {
		return err
	}

	response.NoContent(w)
	return nil
}
